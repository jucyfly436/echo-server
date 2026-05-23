# Echo Server 部署 Runbook

## 阶段一：本地开发环境搭建

### 1. 初始化 Go 模块

```bash
# 创建项目目录
mkdir go-redis-api
cd go-redis-api

# 初始化 Go 模块
go mod init go-redis-api

# 注入代理（否则无法访问 GitHub）
$env:HTTP_PROXY = "http://127.0.0.1:7897"
$env:HTTPS_PROXY = "http://127.0.0.1:7897"

# 安装依赖
go get github.com/go-redis/redis/v8
go get github.com/gorilla/mux
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

### 2. 创建并编写 `main.go`

### 3. 启动本地 Redis

```bash
# 先启动守护进程，再拉取 Redis 7 镜像
docker run -d --name redis -p 6379:6379 redis:7
```

**Linux 环境**下设置环境变量：

```bash
export REDIS_HOST=localhost
export REDIS_PORT=6379
export REDIS_PASSWORD=""  # 如果没有密码
```

**Windows 环境**下推荐使用 `.env` 文件（`$env` 修改的环境变量不是永久的）：

```env
REDIS_HOST=127.0.0.1
REDIS_PORT=6379
REDIS_PASSWORD=your_password
```

```bash
# 安装 dotenv 依赖
go get github.com/joho/godotenv
```

> ⚠️ **注意事项**
>
> - **不要将 `.env` 提交到 Git**：`.env` 文件通常包含敏感信息（如密码），应在 `.gitignore` 中添加 `.env`。
> - **提供模板文件**：创建 `.env.example`，只写键名不写敏感值，方便其他人了解所需环境变量。

### 4. 运行程序并测试端点

```bash
# 测试 greet
curl "http://localhost:8080/api/greet?name=guoying"

# 测试 healthz
curl -i http://localhost:8080/healthz

# 测试 readiness
curl -i http://localhost:8080/readiness

# 测试 version
curl http://localhost:8080/version

# 测试 metrics
curl http://localhost:8080/metrics

# 查询所有访问者
curl http://localhost:8080/api/visitors

# 检查 Redis 数据
docker exec -it redis redis-cli
LRANGE visitors 0 -1
```

### 5. 新增 `/api/visitors` 端点和前端 Dashboard

**`/api/visitors` 接口：**

- 路径：`GET /api/visitors`
- 功能：返回 Redis 中 `visitors` 列表的所有访问者
- 依赖：需要 Redis 连接正常
- 响应示例：

```json
{"visitors":["GuoYing","Tom","World"]}
```

> 如果 Redis 不可用，返回 `500` 和错误信息。

Handler 实现：

```go
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
```

**前端 Dashboard：**

`static/index.html` 提供了一个独立的监控面板页面，服务启动后浏览器访问 `http://localhost:8080/` 即可打开。面板包含：服务状态卡片（Health / Readiness / Version）、API 交互测试区、访问者列表、端点速查。Go 端通过 `http.ServeFile` + `http.FileServer` 提供静态文件服务，Dockerfile 中需将 `static/` 目录复制到运行镜像。

---

## 阶段二：容器化

### 1. 编写 Dockerfile（多阶段构建）

### 2. 编写 `docker-compose.yml`

### 3. 启动服务

```bash
docker compose up --build
```

> **注意**：上一阶段创建的 Redis 容器会占用 `redis` 这个 container_name 和 `6379` 端口，需先删除。
>
> 同时 `.env` 文件中的 `REDIS_HOST` 需改为 `redis`，因为在 Docker Compose 中容器间通信需要通过容器名，而非 `localhost`。

### 4. 端口测试

```bash
# 有改动时重新构建
docker compose down
docker compose up --build
```

---

## 阶段三：CI/CD 流水线

### 1. 创建 `.gitlab-ci.yml`

在项目根目录创建该文件。

### 2. 将代码推送到 GitLab

```bash
# 初始化本地仓库并关联远程仓库
git init
git remote add origin <URL>
git branch -M main

# 验证关联是否成功
git remote -v
```

**推送前检查 `.gitignore`**，至少包含以下内容：

```gitignore
# 环境变量文件（包含敏感密码）
.env

# Go 二进制文件
main
main.exe

# 操作系统临时文件
.DS_Store
Thumbbs.db
```

```bash
# 正式推送
git add .
git commit -m "feat: 添加 Dockerfile 和 gitlab-ci 流水线配置"
git push -u origin main  # -u 关联分支；用 git branch 查看分支名
```

若推送失败，使用 Token 重新关联：

```bash
# 删除旧的远程关联
git remote remove origin

# 使用 Token 重新关联，格式：https://oauth2:TOKEN@gitlab.com/用户名/项目名.git
git remote add origin https://oauth2:<Your_Token>@gitlab.com/piggod-group/piggod-project.git
```

### 3. 手动创建 GitLab Runner（Pipeline 失败时）

**启动 Runner 容器：**

```bash
docker run -d --name my-gitlab-runner --restart always \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v gitlab-runner-config:/etc/gitlab-runner \
  gitlab/gitlab-runner:latest
```

**注册 Runner：**

```bash
docker exec -it my-gitlab-runner gitlab-runner register
```

注册时填写：

- **GitLab instance URL**：`https://gitlab.com/`
- **Registration token**：粘贴 `glrt-xxxx`
- **Name for the runner**：随便起，如 `my-docker-worker`
- **Executor**：**重要，输入 `docker`**
- **Default Docker image**：`alpine:latest`

**排查 Pipeline 失败：**

```bash
# 查看 Runner 日志
docker logs my-gitlab-runner

# 修改 config.toml 开启 privileged 模式
docker exec -it my-gitlab-runner bash
sed -i 's/privileged = false/privileged = true/g' /etc/gitlab-runner/config.toml

# 重启 Runner
docker restart my-gitlab-runner

# 清理残余镜像
docker image prune -a
```

---

## 阶段四：Kubernetes 部署

### 1. 准备清单文件

| 文件                  | 用途                                     |
| --------------------- | ---------------------------------------- |
| `configmap.yaml`      | 解耦配置                                 |
| `deployment.yaml`     | 采用 Sidecar 模式（一个 Pod 跑两个容器） |
| `service.yaml`        | 将应用暴露出来                           |
| `servicemonitor.yaml` | Prometheus Operator 自定义资源           |

### 2. 部署前准备

```bash
# 启动 Minikube
minikube start --preload=false

# 查看环境状态
minikube status

# 添加 Helm 仓库
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

# 创建监控命名空间
kubectl create namespace monitoring

# 安装 kube-prometheus-stack
helm install prometheus prometheus-community/kube-prometheus-stack -n monitoring
```

### 3. 部署应用

```bash
# 业务部署在默认命名空间
kubectl apply -f k8s/
```

> **说明**：Prometheus Operator 会扫描所有命名空间的 ServiceMonitor，只要标签正确，Operator 会自动发现并同步配置，因此 ServiceMonitor 可以 apply 在默认命名空间而非 monitoring。

**创建镜像仓库认证凭据（无访问权限时）：**

```bash
kubectl create secret docker-registry gitlab-registry-key \
  --docker-server=registry.gitlab.com \
  --docker-username=<your-username> \
  --docker-password=<your-token> \
  --docker-email=<your-email>

# 在 deployment 中引用 imagePullSecrets 后重新 apply
kubectl apply -f k8s/deployment.yaml
```

```bash
# 查看 Secret 详细内容（需解码）
kubectl get secret gitlab-registry-key -o yaml

# 强制重试部署
kubectl rollout restart deployment echo-server

# 建立端口转发：本地 8080 → Service 80
kubectl port-forward svc/echo-server-service 8080:80
```

### 4. 验证部署

**访问应用：**

```
http://localhost:8080/api/greet?name=果蝇
```

**测试 Redis 连接：**

```bash
# 进入 echo-app 容器（Pod 有两个容器，需指定名字）
kubectl exec -it <Pod名字> -c echo-app -- /bin/sh

# 在容器里 ping Redis
ping 127.0.0.1
```

**检查 Prometheus 监控：**

```bash
# 转发 Prometheus 界面
kubectl port-forward svc/prometheus-kube-prometheus-prometheus -n monitoring 9090:9090
```

访问 `http://localhost:9090/targets` 查看指标状态。

**资源不足时精简安装 Prometheus：**

```bash
# 卸载旧的
helm uninstall prometheus -n monitoring

# 精简参数安装
helm install prometheus prometheus-community/kube-prometheus-stack -n monitoring \
  --set alertmanager.enabled=false \
  --set grafana.enabled=false \
  --set prometheus.prometheusSpec.resources.requests.memory=256Mi \
  --set prometheus.prometheusSpec.resources.limits.memory=512Mi \
  --set nodeExporter.enabled=false \
  --set kubeStateMetrics.enabled=false

# 关闭 Minikube 非必要服务
minikube addons disable dashboard
minikube addons disable storage-provisioner
```

### 5. 创建并推送部署仓库

```bash
# 在 k8s 目录下执行
git init
git add .
git commit -m "feat: initial k8s manifests for echo-server"
git remote add origin https://gitlab.com/<your-username>/echo-server-deploy.git
git branch -M main
git push -uf origin main
```

---

## 阶段五：声明式部署（ArgoCD）

### 1. 安装 ArgoCD

```bash
# 创建命名空间
kubectl create namespace argocd

# 安装 ArgoCD 核心组件
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
```

### 2. 登录 ArgoCD 控制台

```bash
# 暴露端口
kubectl port-forward svc/argocd-server -n argocd 8081:443

# 获取初始密码（PowerShell）
[System.Text.Encoding]::UTF8.GetString(
  [System.Convert]::FromBase64String(
    (kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}")
  )
)
```

访问 `https://localhost:8081`，在 ArgoCD 中连接仓库。

### 3. 验证 ArgoCD 功能

**验证自愈（Self-Heal）：**

```bash
kubectl scale deployment echo-server --replicas=2
# 观察到副本数变成 2，然后迅速变回 1
```

**验证全链路闭环：**

在 `echo-server-deploy` 仓库编辑 `deployment.yaml` 并推送，ArgoCD 自动同步后旧 Pod 逐渐消失，新 Pod 启动。

**一键回滚（Rollback）：**

1. 在 ArgoCD 界面找到 `HISTORY AND ROLLBACK` 按钮
2. 选择上一个绿色版本，点击 `Rollback`

> **注意**：开启 Self-Heal 时，ArgoCD 会因 Git 未变更而覆盖回滚。极度紧急时，应先关闭 Self-Heal 手动回滚，修好 Bug 后再通过 Git 正常更新，或使用 `git revert`。

---

## 阶段六：配置监控（Grafana）

```bash
# 升级安装，开启 Grafana 并限制内存
helm upgrade prometheus prometheus-community/kube-prometheus-stack -n monitoring \
  --reuse-values \
  --set grafana.enabled=true \
  --set grafana.resources.limits.memory=512Mi \
  --set grafana.resources.requests.memory=256Mi

# 获取 Grafana 密码（PowerShell）
[System.Text.Encoding]::UTF8.GetString(
  [System.Convert]::FromBase64String(
    (kubectl get secret --namespace monitoring -l app.kubernetes.io/component=admin-secret -o jsonpath="{.items[0].data.admin-password}")
  )
)

# 暴露 Grafana 端口
kubectl port-forward svc/prometheus-grafana -n monitoring 3000:80

# 获取 Prometheus 集群内部地址
kubectl get svc -n monitoring
```

**为服务端口添加名称（以便 Prometheus 自动发现）：**

```bash
git add service.yaml
git commit -m "fix: 为服务端口添加名称，以便 Prometheus 自动发现"
git push origin main
```

---

## 项目优化（5-20）

### 优化项总结

| 优先级 | 优化项                          | 原因                                                        |
| ------ | ------------------------------- | ----------------------------------------------------------- |
| 🔴 高   | 补充测试用例                    | 目前只测了 `healthHandler`，其他核心函数出 Bug CI 发现不了  |
| 🔴 高   | Redis 启动加重试                | K8s 里 Pod 启动顺序无法保证，Redis 未就绪时 go-app 直接报错 |
| 🟡 中   | CI 限制只有 main 分支才构建镜像 | 多分支开发时，功能分支代码会覆盖镜像仓库的 latest           |
| 🟡 中   | Dockerfile 的 alpine 固定版本号 | latest 今天和明天拉到的镜像可能不同，导致构建结果不可预期   |
| 🟢 低   | 添加 `.dockerignore`            | 防止 `.env` 密码打进镜像，同时减小构建体积                  |

> **提交建议**：每个优化单独一次 commit，出问题时方便精确定位和回滚。

### 1. 补充测试用例

**问题**：`greetHandler` 里调用了 `rdb.LPush`，但单元测试时 `main()` 不会执行，`rdb` 是 `nil`，直接调用会 **panic**，需要在测试里初始化 `rdb`。

**测试覆盖情况：**

| Handler            | 状态                                               |
| ------------------ | -------------------------------------------------- |
| `healthHandler`    | ✅ 已有测试                                         |
| `greetHandler`     | ❌ 缺失，需测：有 name 参数、无 name 参数默认 World |
| `versionHandler`   | ❌ 缺失，需测：返回码、JSON 格式、version 字段      |
| `readinessHandler` | ❌ 依赖真实 Redis，本地单元测试跳过，CI 环境可测    |

```bash
# 运行测试
go test -v ./...
```

测试通过后的输出：

```
=== RUN   TestMain
=== RUN   TestHealthz
--- PASS: TestHealthz
=== RUN   TestGreetWithName
--- PASS: TestGreetWithName
=== RUN   TestGreetWithoutName
--- PASS: TestGreetWithoutName
=== RUN   TestGreetContentType
--- PASS: TestGreetContentType
=== RUN   TestVersion
--- PASS: TestVersion
PASS
```

```bash
git add main_test.go
git commit -m "test: 添加 greetHandler 和 versionHandler 测试用例"
git push
```

查看流水线 `test_job` 是否通过。

### 2. Redis 启动加重试

修改代码后验证：

```bash
docker compose up
```

正常启动日志应出现：

```
go-app  | 正在连接 Redis...
go-app  | Redis 连接成功
go-app  | Server running on :8080
```

**常见问题**：镜像未重新构建导致重试逻辑不生效。

```bash
# 查看当前 go-app 镜像构建时间（PowerShell）
docker images --format "table {{.Repository}}\t{{.Tag}}\t{{.CreatedAt}}"

# 只重新构建 app，不重启 redis
docker compose up --no-deps --build app
```

> **规律总结**：
> - 改了代码 → 必须重新构建镜像 → `docker compose up --build`
> - 只改了配置（`docker-compose.yml`）→ 不需要重新构建 → `docker compose up`

```bash
git add main.go
git commit -m "fix: add Redis retry on startup"
git push
```

### 3. CI 限制只有 main 分支才构建镜像

在 `build-image-job` 中添加 rules：

```yaml
rules:
  - if: $CI_COMMIT_BRANCH == "main"   # 只有 main 分支才执行这个 job
```

```bash
git add .gitlab-ci.yml
git commit -m "ci: restrict build job to main branch only"
git push
```

建测试分支验证限制：

```bash
git checkout -b test/ci-rules
git push origin test/ci-rules
```

GitLab 流水线正常结果：

```
test-job          ✅ passed
build-image-job   ⏭️ skipped
```

验证完后删除分支：

```bash
git checkout main
git branch -d test/ci-rules
git push origin --delete test/ci-rules
```

### 4. 添加 `.dockerignore`

```dockerignore
.git
.env
docker-compose.yml
.gitlab-ci.yml
*.md
```

验证效果：

```bash
docker compose build app
# 观察 build context 大小是否变小
# => [internal] load build context
# => => transferring context: 223B  ← 变小了
```

```bash
git add .dockerignore
git commit -m "chore: add .dockerignore"
git push
```

### 5. Alpine 镜像版本说明

使用 `CGO_ENABLED=0` 编译生成**纯静态二进制**，不依赖系统库，alpine 版本升级影响极小，暂不需要紧急处理。

| 编译方式                      | alpine 版本变化影响 |
| ----------------------------- | ------------------- |
| 动态二进制（`CGO_ENABLED=1`） | 影响大，可能崩溃    |
| 静态二进制（`CGO_ENABLED=0`） | 影响极小            |

---

## K8s 优化项

### 1. 为 Redis Sidecar 添加资源限制（`resources`）

### 2. 添加健康检查探针

**没有探针的问题：**

```
Pod 启动中，Redis 还没就绪
        │
        ▼
K8s 不知道，直接把流量转发进来   ← 用户收到报错
        │
Pod 内部程序崩溃
        │
        ▼
K8s 不知道，还在转发流量         ← 用户一直报错
```

**加了探针后：**

- `readinessProbe` 失败 → K8s 暂停转发流量，等就绪
- `livenessProbe` 失败 → K8s 自动重启 Pod

**两种探针分工：**

| 探针                         | 检查端点                      | 失败动作             |
| ---------------------------- | ----------------------------- | -------------------- |
| `livenessProbe`（存活探针）  | `/healthz`                    | 直接重启 Pod         |
| `readinessProbe`（就绪探针） | `/readiness`（会 Ping Redis） | 暂停转发流量，不重启 |

**改完后部署：**

```bash
# 确认连接到正确集群
kubectl config current-context

# 按顺序应用（ConfigMap 要最先）
kubectl apply -f configmap.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
kubectl apply -f servicemonitor.yaml

# 查看部署状态
kubectl get pods
kubectl describe pod <pod名字>
```

**镜像拉取失败时（认证失败）：**

```bash
# 确认 Secret 是否存在
kubectl get secret gitlab-registry-key

# 用新 Token 重新创建 Secret
kubectl create secret docker-registry gitlab-registry-key \
  --docker-server=registry.gitlab.com \
  --docker-username=<gitlab-username> \
  --docker-password=<new-token>

# 重新触发镜像拉取
kubectl rollout restart deployment echo-server

# 确认无误后获取访问地址
kubectl get service echo-server-service
```

**重新设置远程仓库地址（Token 更新后）：**

```bash
git remote set-url origin https://<username>:<new-token>@gitlab.com/<group>/<repo>.git
git push
```

**验证探针是否生效：**

```bash
kubectl describe pod <pod名字> | Select-String "Liveness" -Context 0,10
kubectl describe pod <pod名字> | Select-String "Readiness" -Context 0,10
```
## 场景一总结：压力测试

---

### 演练目标
```
模拟服务突然收到大量请求
观察 Grafana 图表的实时变化
验证监控系统能捕捉到流量波动
```

---

### 演练步骤

**第一步：确认服务正常**
```powershell
Invoke-WebRequest -Uri "http://127.0.0.1:64920/healthz"
# 返回 ok 才继续
```

**第二步：开启 Grafana 监控**
```powershell
kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80
```
打开 `localhost:3000`，盯着两个图表。

**第三步：执行压力测试脚本**
```powershell
.\stress.ps1
```

**第四步：观察图表变化**
```
每分钟请求速率：从 0 飙升到 2
P99 请求耗时：  出现数据点，约 4ms
```

---

### 面试时怎么说
```
"这里我模拟了服务突然收到大量请求的场景，
可以看到请求速率图表瞬间飙升，
P99 耗时保持在 4ms，
说明服务在压力下性能表现稳定，
监控系统也成功捕捉到了这次流量波动。"
```

---

## 场景二：模拟 Redis 故障 & 新增指标

---

### 背景

`greetHandler` 里 Redis LPush 失败时只打了日志，仍然返回 200，属于静默降级。为了让 Prometheus 直接暴露 Redis 写失败次数，新增了 `redis_lpush_errors_total` 计数器。

### 改动说明

**改动文件：** `go-redis-api/main.go`、`go-redis-api/main_test.go`

**改动内容：**

1. 新增 Prometheus Counter 指标 `redis_lpush_errors_total`，与 `http_requests_total`、`http_request_duration_seconds` 同级注册
2. `greetHandler` 中 LPush 失败时，计数器 +1（原来只打日志）
3. 新增单元测试 `TestRedisErrorsCounter`，验证指标已注册且 Redis 不可用时不 panic

> 指标含义：`redis_lpush_errors_total` 累计记录 Redis LPUSH 失败次数，重启后归零。Grafana 面板上可基于此指标设置告警。

### 演练步骤

**第一步：部署更新后的应用**

```bash
# 重新构建镜像并推送到 GitLab Registry
docker build -t registry.gitlab.com/<group>/<repo>:latest ./go-redis-api
docker push registry.gitlab.com/<group>/<repo>:latest

# K8s 重新拉取镜像
kubectl rollout restart deployment echo-server
```

**第二步：建立端口转发并确认新指标存在**

```bash
kubectl port-forward svc/echo-server-service 8080:80
```

访问 `http://localhost:8080/metrics`，搜索 `redis_lpush_errors_total`，应看到初始值为 0。

**第三步：模拟 Redis 故障（CLIENT PAUSE）**

```bash
POD=$(kubectl get pods -l app=echo-server -o jsonpath='{.items[0].metadata.name}')
kubectl exec $POD -c redis-sidecar -- redis-cli CLIENT PAUSE 60000
# 60 秒内 Redis 不响应任何命令，但进程不退出
```

**第四步：发送请求触发 LPush 错误**

```bash
for i in $(seq 1 10); do
  curl -s "http://localhost:8080/api/greet?name=Tom" &
done
```

**第五步：验证计数器变化**

```bash
curl -s http://localhost:8080/metrics | grep redis_lpush_errors_total
# redis_lpush_errors_total 10  ← LPush 失败 10 次，每封请求仍返回 200
```

### 面试时怎么说

```
"之前 Redis 写失败只会打日志，Prometheus 感知不到。
我在 greetHandler 里加了一个 redis_lpush_errors_total 计数器，
Redis LPush 出错时自动 +1，现在可以在 Grafana 面板上直接看到
写入失败的次数，也能基于这个指标设置告警。"
```

---

## 场景二执行总结

---

### 实际执行记录（2026-05-23）

**遇到的问题：**

1. CI `build-image-job` 报错 `lookup docker: no such host` — GitLab Runner 的 DinD 服务别名无法解析
2. 修复方式：在 `.gitlab-ci.yml` 的 `build-image-job` 里显式添加 `DOCKER_HOST: tcp://docker:2375` 变量

**修复涉及的 commit：**

| 提交                                             | 内容                        |
| ------------------------------------------------ | --------------------------- |
| `chore：新增 redis_lpush_errors_total 计数器...` | main.go / main_test.go 改动 |
| `fix：build-image-job 显式指定 DOCKER_HOST 变量` | .gitlab-ci.yml 修复         |

**部署后验证：**

> 注意：`CounterVec` 在未被 `.Inc()` 驱动前不会出现在 `/metrics` 输出中，需先触发 Redis 故障才能看到该指标。

```bash
# 1. CI 完成后重启 K8s 拉取新镜像
kubectl rollout restart deployment echo-server

# 2. 建立端口转发
kubectl port-forward svc/echo-server-service 8080:80

# 3. 冻结 Redis 触发 LPush 错误
POD=$(kubectl get pods -l app=echo-server -o jsonpath='{.items[0].metadata.name}')
kubectl exec $POD -c redis-sidecar -- redis-cli CLIENT PAUSE 30000

# 4. 发送请求并检查指标
curl -s "http://localhost:8080/api/greet?name=Tom"
curl -s http://localhost:8080/metrics | grep redis_lpush_errors_total
# 输出: redis_lpush_errors_total 1
```

**结论：** 新指标 `redis_lpush_errors_total` 成功注册并正常驱动，Redis 写入失败时计数器自增，Prometheus 可采集到该指标用于告警。`greetHandler` 在 Redis 不可用时仍返回 200（静默降级），错误通过日志和指标共同暴露。

---

## 场景三：Pod 崩溃自动重启

**目标：** 模拟主应用崩溃，观察 K8s liveness 探针如何触发自动重启。

---

### 先确认当前状态

```powershell
kubectl get pods
```

记下当前的 RESTARTS 次数，等会对比用。

---

### 让 echo-app 进程崩溃

```powershell
# 进入 echo-app 容器，直接杀掉主进程
kubectl exec -it echo-server-857c4bbc95-m2spf -c echo-app -- sh

# 进去之后执行
kill 1
```

`kill 1` 是杀掉容器内 PID 为 1 的进程，也就是你的 Go 程序本身。

---

### 立刻观察

新开一个 PowerShell 窗口：

```powershell
kubectl get pods -w
```

```
kill 1 杀掉 Go 进程
    │
    ▼
echo-app 容器退出（不是整个 Pod）
    │
    ▼
K8s 检测到容器退出
    │
    ▼
自动重启 echo-app 容器
    │
    ▼
RESTARTS 从 0 变成 1
    │
    ▼
Redis 还在运行（sidecar 没有受影响）
    │
    ▼
echo-app 重启后连上 Redis，恢复 2/2 Running
```

---

### 和场景二的关键区别

```
场景二（Redis 故障）：
    readiness 失败 → 暂停流量 → Pod 不重启
    依赖恢复后自动重新接收流量

场景三（进程崩溃）：
    进程直接退出 → K8s 检测到 → 立刻重启容器
    不需要等 liveness 探针失败
    因为进程退出比探针检测更直接
```

---

## 面试时怎么说

```
"这里我模拟了主应用进程崩溃的场景，
K8s 检测到容器退出后立刻重启，
RESTARTS 从 0 变成 1，
整个恢复过程只需要几秒钟，
Redis sidecar 完全不受影响，
这就是 K8s 自愈能力的体现。"
```

---

## 三个场景全部完成总结

```
场景一：压力测试
    → 监控捕捉到流量波动，P99 保持 4ms，服务稳定

场景二：Redis 故障
    → readiness 探针失败，K8s 暂停流量，进程不重启
    → 依赖恢复后自动重新接收流量

场景三：进程崩溃
    → K8s 立刻重启容器，RESTARTS+1
    → 几秒内自动恢复
```

### 这三个场景串起来，完整展示了：

```
监控能力  → 场景一，看到流量变化
故障隔离  → 场景二，探针保护用户不受影响
自愈能力  → 场景三，崩溃自动恢复
```

---