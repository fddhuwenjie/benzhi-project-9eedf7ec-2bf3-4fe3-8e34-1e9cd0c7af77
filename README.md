# 航空发动机试车数据有效性裁定台

本项目为试车数据工程师、试验平台主管和独立质量复核员提供浏览器工作台。系统覆盖试车建档、测点基线冻结、采集数据质量门禁、异常分诊、处置证据、独立复核、有效性裁定及只读档案下载，并记录带哈希链的全过程审计事件。

## 状态流程

主流程为 `DRAFT -> BASELINED -> DATA_CHECKED -> TRIAGED -> REVIEW_PENDING -> REVIEWED -> DECIDED -> ARCHIVED`。复核退回时从 `REVIEW_PENDING` 返回 `TRIAGED`；处置提交人与复核员必须是不同身份。所有写请求使用 `X-Request-ID` 幂等标识和 `expected_revision` 乐观并发版本。

## 构建与运行

```bash
go build ./cmd/server
go run ./cmd/server -addr=127.0.0.1:19081 -db=test_runs.db
```

未指定 `-addr` 时，服务优先读取 `PORT` 并绑定 `127.0.0.1:<PORT>`，否则默认监听 `127.0.0.1:19081`。浏览器访问 `http://127.0.0.1:19081/`。`-db` 指向 SQLite 数据库；存储启用 WAL、外键和忙等待配置，聚合、幂等结果与只追加审计事件在同一事务快照中提交。

## 测试与自检

```bash
go test ./...
go run ./cmd/server -self-check -addr=127.0.0.1:19081
```

自检创建临时数据文件，启动真实回环 HTTP 监听器，通过 JSON API 完成建档、冻结、数据登记、异常处置、独立复核、裁定和封存，并验证审计链后主动退出。

## API 概览

页面入口为 `GET /`，就绪检查为 `GET /readyz`。业务 API 以 `/api/runs` 为根，提供可组合筛选、稳定游标分页、阶段统计与详情，并通过 `/baseline`、`/package`、`/triage`、`/evidence`、`/review`、`/decision`、`/archive` 推进状态。

草稿可通过 `PATCH /api/runs/{id}` 按 revision 修订，`GET /api/runs/{id}/baseline` 返回规范化测点、保存差异和候选摘要，冻结时回传该摘要。数据包入口接收完整 `files`、`channel_summaries`、采集窗口、漂移和重复片段；`PUT /api/runs/{id}/package` 只读预演门禁与预计异常，确认登记时以 `candidate_package_hash` 和 `expected_revision` 锁定预演载荷。

`GET /api/runs/{id}/anomaly-impact` 提供冻结测点矩阵、重叠组、稳定优先清单和组合筛选。`GET /api/runs/{id}/evidence` 执行闭环预检，`POST` 的 `items` 支持证据原子批量提交与严格取代链。`GET /api/runs/{id}/review` 返回最新退回基准的再审差异；再次复核以 `comparison_review_id` 引用比较基准。分诊仍支持原单项输入以及原子 `items` 数组。

`GET /api/runs/{id}/decision` 返回只读裁定就绪检查，`GET /api/runs/{id}/timeline` 返回审计时间线。封存后 `GET /api/runs/{id}/archive?report=true` 返回完整性报告，`GET /api/runs/{id}/archive` 下载确定性 JSON，响应带 `ETag` 与文件名并支持 `If-None-Match`。

`GET /api/archives` 仅检索 `ARCHIVED` 聚合，支持试车台、发动机、裁定、签发人、签发时间、适用目标和限制条件组合筛选，返回筛选统计与稳定游标分页。工作台中的“封存档案”标签页使用同一 API，并沿用现有详情、时间线和下载入口。
