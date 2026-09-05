# 更新日志

> 本文为 [CHANGELOG.md](CHANGELOG.md) 的中文译本。

本项目的所有重要变更都会记录在此文件中。

格式遵循 [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)，
版本号遵循 [语义化版本](https://semver.org/spec/v2.0.0.html)。

*说明*：更新日志只收录对业务和服务运行有意义的变更。  
例如文档修正、lint 修复等 **不应** 写入本文件。

## [0.2.5] - 2019-03-13

- 修复限价单的成交价格

## [0.2.1] - 2019-03-13

- 新增 Depth 方法，用于获取买盘与卖盘的价格档位
- 新增 Order 方法，用于按 ID 获取 Order 对象

## [0.2.0] - 2019-03-13

- 市价单和限价单新增部分成交数量（partial quantity processed）返回值

## [0.1.0] - 2019-03-01

- 接入 Travis CI 与 GolangCI linter
- 修复 go vet 警告
- 新增 json.Marshaler 与 json.Unmarshaler 接口
- 新增按指定数量计算市价总额（CalculateMarketPrice）

## [0.0.1] - 2019-02-17

orderbook 库的首次发布。  
此前该功能已作为库使用。

- 标准价格-时间优先
- 同时支持市价单和限价单
- 支持撤单
- 高性能（每秒超过 30 万笔成交）
- 内存占用优化
