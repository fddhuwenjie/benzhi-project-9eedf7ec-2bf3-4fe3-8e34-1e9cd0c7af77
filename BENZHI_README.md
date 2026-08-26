# BENZHI_README

## 项目说明
- 项目：benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77
- 项目用途：已实现航空发动机试车数据有效性裁定浏览器工作台，覆盖基线冻结、数据质量门禁、异常处置、独立复核、有效性裁定、不可变档案和哈希链审计，并提供可自行结束的真实 HTTP 全流程自检。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：航空发动机试车数据有效性裁定台
- 项目介绍：面向试车数据工程师和独立复核员的浏览器工作台，将一次航空发动机地面试车从基线建档、采集数据校验、异常处置推进到有效性裁定与不可变封存，确保每个结论都能追溯至原始数据摘要、处置证据和复核意见。
- 项目概述：面向试车数据工程师和独立复核员的浏览器工作台，将一次航空发动机地面试车从基线建档、采集数据校验、异常处置推进到有效性裁定与不可变封存，确保每个结论都能追溯至原始数据摘要、处置证据和复核意见。
- 核心工作流：试车任务以草稿状态建档并冻结测点基线，登记采集数据包后执行完整性与时序校验；工程师逐项分诊异常、提交处置证据并申请独立复核，复核退回时回到异常处置状态，复核通过后签发试验有效或无效裁定，最终生成摘要清单并封存为只读档案。
- 对外接口：Go 服务提供原生 HTML、CSS 和 JavaScript 的浏览器工作台及同源 JSON API，页面覆盖试车列表、测点校验、异常处置、独立复核和裁定档案视图；服务支持 -addr=127.0.0.1:<port>，也支持用 PORT 端口号绑定 127.0.0.1:<PORT>，默认监听 127.0.0.1:19081，禁止默认绑定 0.0.0.0 或常见低位端口。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/server -self-check -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77-arm64 linux/arm64

docker run -it benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -self-check -addr=127.0.0.1:19081`
