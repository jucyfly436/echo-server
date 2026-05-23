package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-redis/redis/v8"
)

// TestMain 是测试的入口，在所有测试用例执行前运行
// 用来初始化 rdb，防止 greetHandler 里调用 rdb.LPush 时 panic
func TestMain(m *testing.M) {
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // 单元测试环境可能没有 Redis
	})                         // 连接失败没关系，greetHandler 会 log 错误但不会崩溃
	os.Exit(m.Run())
}

// -------- healthHandler --------
// 测试工具只运行以 test 开头的函数
func TestHealthz(t *testing.T) {
	req := httptest.NewRequest("GET", "/healthz", nil)	//伪造一个HTTP请求，方法为GET，路径为/healthz，请求体为nil
	w := httptest.NewRecorder()	//伪造一个HTTP响应记录器，用于捕获处理程序的响应

	healthHandler(w, req)	//调用healthHandler函数，传入伪造的请求和响应记录器

	if w.Code != http.StatusOK {	//从记录器中获取HTTP状态码，如果不是200 OK，则测试失败
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got '%s'", w.Body.String())
	}
}

// -------- greetHandler --------

func TestGreetWithName(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/greet?name=Tom", nil)
	w := httptest.NewRecorder()

	greetHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["message"] != "Hello, Tom!" {
		t.Errorf("expected 'Hello, Tom!', got '%s'", resp["message"])
	}
}

func TestGreetWithoutName(t *testing.T) {
	// 不传 name 参数，应该默认返回 Hello, World!
	req := httptest.NewRequest("GET", "/api/greet", nil)
	w := httptest.NewRecorder()

	greetHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["message"] != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got '%s'", resp["message"])
	}
}

func TestGreetContentType(t *testing.T) {
	// 验证响应头是 application/json
	req := httptest.NewRequest("GET", "/api/greet?name=Tom", nil)
	w := httptest.NewRecorder()

	greetHandler(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

// -------- versionHandler --------

func TestRedisErrorsCounter(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/greet?name=Tom", nil)
	w := httptest.NewRecorder()

	greetHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestVersion(t *testing.T) {
	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()

	versionHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 验证 Content-Type
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	// 验证 version 字段存在且不为空
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["version"] == "" {
		t.Errorf("expected version field to be non-empty")
	}
}