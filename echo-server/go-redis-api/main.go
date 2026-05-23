package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"	//路由库
	"github.com/joho/godotenv" // 读取 .env 文件
	"github.com/prometheus/client_golang/prometheus"	//监控指标库
	"github.com/prometheus/client_golang/prometheus/promhttp"	//指标暴露端点
)

var (
	ctx = context.Background()
	rdb *redis.Client

	version = "v1.0.0" // 可以通过构建注入

	redisHost string
  redisPort string
)

var (
	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"path"},
	)
	redisErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_lpush_errors_total",
			Help: "Total number of Redis LPUSH errors",
		},
		[]string{},
	)
	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path"},
	)
)

func init() {
	prometheus.MustRegister(httpRequests, httpDuration, redisErrors)
}

func main() {
	// 尝试加载 .env 文件
	// 如果文件不存在，Load 会报错。在开发环境下我们通常加载它；
	// 在生产环境（如 Docker/K8s）中，环境变量通常直接注入，不需要 .env 文件。
	err := godotenv.Load()
	if err != nil {
		log.Println("提示: 未找到 .env 文件，将尝试从系统环境变量读取配置")
	}
	
	// 从环境变量读取 Redis 配置
	redisHost = os.Getenv("REDIS_HOST")
	redisPort = os.Getenv("REDIS_PORT")
	redisPassword := os.Getenv("REDIS_PASSWORD")

	// 简单的防御性检查：如果变量为空，设置默认值
	if redisHost == "" {
		redisHost = "localhost"
	}
	if redisPort == "" {
		redisPort = "6379"
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
		Password: redisPassword,
		DB:       0,
	})

	//启动时等待 Redis 就绪，最多重试10次，每次间隔3秒
	log.Println("正在连接 Redis...")
	connected := false
	for i := 1; i <= 10; i++ {
			_, err := rdb.Ping(ctx).Result()
			if err == nil {
					log.Println("Redis 连接成功")
					connected = true
					break
			}
			log.Printf("Redis 未就绪，第 %d 次重试，3秒后再试... (%v)", i, err)
			time.Sleep(3 * time.Second)
	}
	if !connected {		// 10次重试后仍未连接成功，记录错误并退出程序
			log.Fatalf("Redis 连接失败，已重试 10 次，退出程序")
	}

	// 路由
	r := mux.NewRouter()
	r.HandleFunc("/api/greet", withMetrics(greetHandler)).Methods("GET")
	r.HandleFunc("/api/visitors", visitorsHandler).Methods("GET")
	r.HandleFunc("/healthz", healthHandler).Methods("GET")
	r.HandleFunc("/readiness", readinessHandler).Methods("GET")
	r.HandleFunc("/version", versionHandler).Methods("GET")
	r.Handle("/metrics", promhttp.Handler())

	// 前端页面
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/index.html")
	})

	fmt.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// Middleware: Prometheus metrics 中间件，记录请求次数和耗时
func withMetrics(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next(w, r)
		duration := time.Since(start).Seconds()
		httpRequests.WithLabelValues(r.URL.Path).Inc()
		httpDuration.WithLabelValues(r.URL.Path).Observe(duration)
	}
}

// /api/greet?name=World
func greetHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "World"
	}

	// 写入 Redis List 记录访问者
	if err := rdb.LPush(ctx, "visitors", name).Err(); err != nil {
		log.Println("Redis LPUSH error:", err)
		redisErrors.WithLabelValues().Inc()
	}

	resp := map[string]string{"message": fmt.Sprintf("Hello, %s!", name)}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// /healthz 健康检查
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// /readiness 就绪检查，会真正检查 Redis 是否连通
func readinessHandler(w http.ResponseWriter, r *http.Request) {
    _, err := rdb.Ping(ctx).Result()
    if err != nil {
				log.Printf("无法连接到 Redis: %v (Host: %s, Port: %s)", err, redisHost, redisPort)	//把错误打印出来，这样在 GitLab 日志里就能看到了
        // 直接把具体的错误返回给网页，方便你调试
        errMsg := fmt.Sprintf("Redis 连接失败: %v", err)
        http.Error(w, errMsg, http.StatusServiceUnavailable)
        return
    }
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
}

// /api/visitors 获取 Redis 中记录的访问者列表
func visitorsHandler(w http.ResponseWriter, r *http.Request) {
	visitors, err := rdb.LRange(ctx, "visitors", 0, -1).Result()
	if err != nil {
		log.Println("Redis LRANGE error:", err)
		http.Error(w, `{"error":"failed to fetch visitors"}`, http.StatusInternalServerError)
		return
	}
	if visitors == nil {
		visitors = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"visitors": visitors})
}

// /version 获取当前程序版本
func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"version": version})
}