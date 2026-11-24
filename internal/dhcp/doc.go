// Package dhcp 是 DHCP / ProxyDHCP 服务端：报文编解码、应答构造、地址池与 PXE 引导信息下发。
//
// 本包按职责分文件（Go 没有运行时动态加载，包内分文件零导入成本，是这里唯一的
// 「按需加载」手段）；改哪一块只读对应文件即可：
//
//	文件              职责                                          依赖
//	server.go         结构与生命周期（起停、读循环、连接持有）          本包其余文件
//	dispatch.go       报文分发与消息语义（DISCOVER/REQUEST/RELEASE…）  reply.go、pool.go
//	reply.go          应答构造、Option 43、NAK、额外选项覆盖           transport.go
//	transport.go      UDP 收发（绑端口、单播/广播目的地、启动摘要）     network_*.go
//	pool.go           地址池（范围算术、租约表、隔离表、快照）          无（纯内存）
//	log.go            日志级别判定与同客户端 5 秒合并                   internal/logbook
//	packet.go         BOOTP/DHCP 编解码与选项读写（不再细分）            无
//	network_*.go      平台相关（Linux 指定网卡收发、SO_BROADCAST）      golang.org/x/sys
//
// 依赖方向严格单向：server → dispatch → reply → transport，log / pool 被上述文件调用
// 而不回调。新增文件必须落在这张图里，不要引入回边。
package dhcp
