# Echo Server — 云原生全链路部署实践

一个从零搭建的 Go HTTP 服务，完整走通**本地开发 → 容器化 → CI/CD → Kubernetes 部署 → GitOps → 监控告警 → 故障演练**全链路，用于系统学习云原生 DevOps 实践。

## 技术栈

| 分类 | 技术 |
|------|------|
| 后端服务 | Go 1.21、gorilla/mux、go-redis、Prometheus SDK |
| 容器化 | Docker（多阶段构建）、Docker Compose |
| CI/CD | GitLab CI/CD、GitLab Container Registry |
| 编排部署 | Kubernetes（Minikube）、Helm |
| GitOps | ArgoCD |
| 可观测性 | Prometheus、Grafana、kube-prometheus-stack |

## 架构概览

```
┌─────────────────────────────────────────────────────────┐
│                      GitLab                             │
│   代码推送 → CI 测试 → 构建镜像 → 推送 Registry         │
└───────────────────────┬─────────────────────────────────┘
                        │ 镜像更新
                        ▼
┌─────────────────────────────────────────────────────────┐
│                    ArgoCD (GitOps)                      │
│   监听部署仓库 → 自动同步 → 自愈 / 一键回滚             │
└───────────────────────┬─────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────┐
│                  Kubernetes Pod                         │
│  ┌──────────────────┐  ┌──────────────────┐            │
│  │   echo-app (Go)  │  │  redis-sidecar   │            │
│  │  :8080           │  │  :6379           │            │
│  │  /api/greet      │  │  访客记录存储    │            │
│  │  /healthz        │  └──────────────────┘            │
│  │  /readiness      │                                  │
│  │  /metrics        │                                  │
│  └──────────────────┘                                  │
└───────────────────────┬─────────────────────────────────┘
                        │ 指标采集
                        ▼
┌─────────────────────────────────────────────────────────┐
│            Prometheus + Grafana                         │
│   http_requests_total / http_request_duration_seconds   │
│   redis_lpush_errors_total（自定义指标）                │
└─────────────────────────────────────────────────────────┘
```

## 服务端点

| 端点 | 说明 |
|------|------|
| `GET /api/greet?name=xxx` | 问候接口，写入 Redis 访客记录 |
| `GET /api/visitors` | 查询所有访客列表 |
| `GET /healthz` | 存活探针（liveness） |
| `GET /readiness` | 就绪探针，检查 Redis 连通性 |
| `GET /metrics` | Prometheus 指标端点 |
| `GET /version` | 版本信息 |
| `GET /` | 前端监控 Dashboard |

## 快速启动

**前置条件：** Docker、Go 1.21+

```bash
# 克隆项目
git clone https://github.com/jucyfly436/echo-server.git
cd echo-server

# 配置环境变量
cp .env.example .env
# 编辑 .env 填写 REDIS_HOST / REDIS_PORT

# 一键启动（应用 + Redis）
docker compose up --build

# 验证服务
curl "http://localhost:8080/api/greet?name=world"
curl http://localhost:8080/healthz
```

## 项目结构

```
echo-server/
├── go-redis-api/
│   ├── main.go              # 服务主体
│   ├── main_test.go         # 单元测试
│   ├── Dockerfile           # 多阶段构建
│   └── static/index.html    # 监控 Dashboard
├── k8s/
│   ├── configmap.yaml
│   ├── deployment.yaml      # Sidecar 模式
│   ├── service.yaml
│   └── servicemonitor.yaml  # Prometheus Operator
├── .gitlab-ci.yml           # CI/CD 流水线
├── docker-compose.yml
└── .env.example
```

## K8s 部署

```bash
# 启动 Minikube
minikube start

# 安装监控栈
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
kubectl create namespace monitoring
helm install prometheus prometheus-community/kube-prometheus-stack -n monitoring

# 部署应用
kubectl apply -f k8s/

# 访问服务
kubectl port-forward svc/echo-server-service 8080:80
```

## 故障演练场景

### 场景一：压力测试
模拟突发流量，验证监控捕捉能力。

```bash
# 执行压力测试后观察 Grafana
# 结果：P99 延迟稳定在 4ms，监控实时捕捉流量波动
```

### 场景二：Redis 故障隔离
通过 `CLIENT PAUSE` 冻结 Redis，验证 readiness 探针的流量拦截。

```bash
POD=$(kubectl get pods -l app=echo-server -o jsonpath='{.items[0].metadata.name}')
kubectl exec $POD -c redis-sidecar -- redis-cli CLIENT PAUSE 30000

# 观察：readiness 探针失败 → K8s 暂停流量转发
# 自定义指标 redis_lpush_errors_total 计数上升
# greetHandler 静默降级，仍返回 200
```

### 场景三：进程崩溃自动重启
Kill 主进程，验证 K8s 自愈能力。

```bash
kubectl exec -it <pod-name> -c echo-app -- kill 1
# 观察：RESTARTS +1，容器秒级重启，Redis sidecar 不受影响
```

## CI/CD 流水线

```
代码推送到 main 分支
    │
    ├─→ test-job：go test ./...
    │
    └─→ build-image-job：docker build + push to GitLab Registry
            （仅 main 分支触发，feature 分支跳过）
```

## 自定义 Prometheus 指标

| 指标名 | 类型 | 说明 |
|--------|------|------|
| `http_requests_total` | Counter | 按路径统计请求总数 |
| `http_request_duration_seconds` | Histogram | 请求耗时分布（P99） |
| `redis_lpush_errors_total` | Counter | Redis 写入失败次数 |# echo-server
