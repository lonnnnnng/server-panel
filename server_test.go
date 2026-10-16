package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestConfigDefaultAndOverrides(t *testing.T) {
	for _, key := range []string{"HOST", "PORT", "SAMPLE_INTERVAL_MS", "MAX_HISTORY"} {
		t.Setenv(key, "")
	}
	cfg, err := loadConfig()
	if err != nil || cfg.Port != "8888" || cfg.Host != "0.0.0.0" || cfg.History != 120 || cfg.Interval != time.Second {
		t.Fatalf("unexpected default config: %+v, %v", cfg, err)
	}
	t.Setenv("PORT", "9001")
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("SAMPLE_INTERVAL_MS", "1250")
	t.Setenv("MAX_HISTORY", "30")
	cfg, err = loadConfig()
	if err != nil || cfg.Port != "9001" || cfg.Host != "127.0.0.1" || cfg.History != 30 || cfg.Interval != 1250*time.Millisecond {
		t.Fatalf("override not applied: %+v, %v", cfg, err)
	}
}

func TestInvalidConfigFailsClosed(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"PORT", "0"}, {"PORT", "65536"}, {"PORT", "invalid"},
		{"SAMPLE_INTERVAL_MS", "-1"}, {"SAMPLE_INTERVAL_MS", "999"}, {"SAMPLE_INTERVAL_MS", "60001"},
		{"MAX_HISTORY", "0"}, {"MAX_HISTORY", "3601"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			for _, key := range []string{"PORT", "SAMPLE_INTERVAL_MS", "MAX_HISTORY"} {
				t.Setenv(key, "")
			}
			t.Setenv(tc.key, tc.value)
			if _, err := loadConfig(); err == nil {
				t.Fatal("invalid configuration must fail instead of silently using defaults")
			}
		})
	}
}

func TestRoutesAndEmbeddedAssets(t *testing.T) {
	state := &AppState{latest: &Metrics{Timestamp: "test-snapshot"}}
	handler := newHandler(state, time.Second, context.Background())
	for _, tc := range []struct {
		path, contentType string
		status            int
	}{
		{"/", "text/html", 200}, {"/index.html", "text/html", 200},
		{"/app.js", "text/javascript", 200}, {"/styles.css", "text/css", 200},
		{"/api/health", "application/json", 200}, {"/api/metrics", "application/json", 200},
		{"/config/server-panel.env", "", 404}, {"/main.go", "", 404},
		{"/public/", "", 404}, {"/api/unknown", "", 404},
	} {
		t.Run(tc.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if recorder.Code != tc.status || !strings.Contains(recorder.Header().Get("Content-Type"), tc.contentType) {
				t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Header().Get("Content-Type"))
			}
			if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing nosniff header")
			}
			if tc.path == "/" && !strings.Contains(recorder.Body.String(), "Server Panel") {
				t.Fatal("embedded page missing application name")
			}
			if tc.path == "/api/health" {
				var result map[string]any
				if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil || result["ok"] != true || result["version"] != version {
					t.Fatalf("invalid versioned health response: %s", recorder.Body.String())
				}
			}
			if tc.path == "/api/metrics" && !strings.Contains(recorder.Body.String(), "test-snapshot") {
				t.Fatal("metrics snapshot not returned")
			}
		})
	}
	for _, path := range []string{"/", "/api/health", "/api/metrics", "/api/events"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s must not be accepted", path)
		}
	}
}

func TestSimplifiedDashboardAssets(t *testing.T) {
	html, err := publicFiles.ReadFile("public/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{`class="stat-strip"`, "pressureCpu", "pressureMemory", "pressureIo", "fileHandles", "processSummary", "tcpTotal"} {
		if strings.Contains(string(html), removed) {
			t.Errorf("removed dashboard content still present: %s", removed)
		}
	}
	for _, heading := range []string{"网络吞吐", "进程热点"} {
		if !strings.Contains(string(html), "<h2>"+heading+"</h2>") {
			t.Errorf("required dashboard section missing: %s", heading)
		}
	}
	for _, id := range []string{"rxRate", "txRate", "rxTotal", "txTotal", "trafficTotal", "packetRate", "topCpuTable", "topMemoryTable"} {
		if !strings.Contains(string(html), `id="`+id+`"`) {
			t.Errorf("required dashboard element missing: %s", id)
		}
	}
	if strings.Contains(string(html), "chart-grid") || strings.Contains(string(html), "<canvas") {
		t.Fatal("trend charts must be removed")
	}
	// long: 开机日期与持续运行时长成对阅读，日期应排在时长之前且只展示一次。
	bootPosition := strings.Index(string(html), `id="bootTimeBadge"`)
	uptimePosition := strings.Index(string(html), `id="uptimeBadge"`)
	if bootPosition < 0 || uptimePosition < 0 || bootPosition >= uptimePosition || strings.Count(string(html), `id="bootTimeBadge"`) != 1 {
		t.Fatal("boot time must appear once before uptime")
	}
	script, err := publicFiles.ReadFile("public/app.js")
	if err != nil {
		t.Fatal(err)
	}
	// long: 页面精简时必须同步清理绑定，避免首次指标更新访问已删除的 DOM 并中断后续刷新。
	bindings := regexp.MustCompile(`byId\('([^']+)'\)`).FindAllStringSubmatch(string(script), -1)
	for _, binding := range bindings {
		if !strings.Contains(string(html), `id="`+binding[1]+`"`) {
			t.Errorf("frontend references missing DOM element: %s", binding[1])
		}
	}
	for _, id := range []string{"hardware", "hostGroups", "localAddresses", "tempZones", "fsList", "memoryDetails", "interfaces", "tcpStates"} {
		if strings.Contains(string(html), `id="`+id+`"`) {
			t.Errorf("removed detail element still present: %s", id)
		}
	}
	for _, heading := range []string{"硬件详情", "主机", "文件系统", "内存细节", "网络细节"} {
		if strings.Contains(string(html), "<h2>"+heading+"</h2>") {
			t.Errorf("removed detail heading still present: %s", heading)
		}
	}
	for _, id := range []string{"cpuValue", "memoryValue", "diskValue", "netErrorsDrops", "tcpEstablished", "tcpListening"} {
		if !strings.Contains(string(html), `id="`+id+`"`) {
			t.Errorf("required dashboard element missing: %s", id)
		}
	}
	if strings.Contains(string(html), "端口占用") || strings.Contains(string(html), `id="portListeners"`) {
		t.Fatal("port occupancy list must be removed")
	}
	if strings.Contains(string(script), "renderPortListeners") || strings.Contains(string(script), "portListeners") {
		t.Fatal("port occupancy frontend code must be removed")
	}
}

func TestMetricsTimestampPreservesSubsecondPrecision(t *testing.T) {
	// long: 秒级格式会早于本轮采集起点，无法区分同一秒内的两次刷新。
	started := time.Now()
	state := &AppState{maxHistory: 1}
	if err := state.collect(); err != nil {
		t.Fatal(err)
	}
	sampled, err := time.Parse(time.RFC3339Nano, state.snapshot().Timestamp)
	if err != nil || sampled.Before(started) || sampled.After(time.Now()) {
		t.Fatalf("sampling timestamp lost precision: %s, %v", state.snapshot().Timestamp, err)
	}
}

func TestSSEStopsWithServiceContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := &AppState{latest: &Metrics{Timestamp: "stream-snapshot"}}
	server := httptest.NewServer(newHandler(state, 5*time.Millisecond, ctx))
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(server.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Type") != "text/event-stream; charset=utf-8" {
		t.Fatal("expected event stream")
	}
	scanner := bufio.NewScanner(response.Body)
	seen := false
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "data: ") {
			seen = strings.Contains(scanner.Text(), "stream-snapshot")
			cancel()
			break
		}
	}
	if !seen {
		t.Fatal("no SSE metrics received")
	}
	for scanner.Scan() {
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("SSE did not end cleanly after shutdown: %v", err)
	}
}

func TestCounterResetAndDiskAggregation(t *testing.T) {
	// long: 网卡计数器重置不能产生负吞吐，物理盘与映射盘不能重复计入汇总。
	if delta(1, 100) != 0 || delta(110, 100) != 10 {
		t.Fatal("counter reset handling is incorrect")
	}
	devices := map[string]DiskDevice{
		"sda": {Name: "sda"}, "dm-0": {Name: "dm-0"},
	}
	selected := selectAggregateDiskDevices(devices)
	if len(selected) != 1 || selected[0].Name != "sda" {
		t.Fatalf("aggregate disk counted mapped devices: %+v", selected)
	}
}
