基于 Go 实现的高层公共建筑薄抹灰岩棉外墙外保温工程 Web 项目，一款后端服务，实现从基层交接、铺板锚固、养护检测到抹面移交的质量闭环。

# rockwool-facade-render-handover

本 Git 项目来自模型完成任务后的 workspace，不包含嵌套 .git 记录或本地构建产物。

## 本地构建与测试

```bash
go mod download
go build ./...
go test ./...
./run_benzhi_smoke.sh
```

## Docker 构建与运行

```bash
docker build --platform linux/amd64 -t rockwool-facade-render-handover:latest .
./build_benzhi_docker.sh rockwool-facade-render-handover linux/arm64
docker run --rm -it --platform linux/arm64 rockwool-facade-render-handover:latest
./build_benzhi_docker.sh rockwool-facade-render-handover linux/amd64
docker run --rm -it --platform linux/amd64 rockwool-facade-render-handover:latest
```

构建脚本第二个参数为目标平台，必须分别完成 linux/arm64 和 linux/amd64 构建与容器验证；未提供时按照规范默认使用 linux/amd64。系统 backend-v2 模板通过 Go 原生交叉编译生成目标架构的 /usr/local/bin/benzhi-app，镜像默认直接运行该入口。
