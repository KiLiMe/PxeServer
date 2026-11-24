// Package config 是配置数据结构、持久化与校验：字段与界面控件一一对应。
//
// 依赖边界：本包只依赖标准库、internal/netaddr 与 internal/netif，绝不能被
// internal/app 或 internal/ui 反向依赖。改「哪个字段怎么校验」只看 normalize.go，
// 改「怎么读写盘」只看 persist.go：
//
//	文件          职责                                        依赖
//	config.go     Config / Runtime 结构与默认值；BootHost、      internal/netaddr
//	              IsReserved 这类派生判断（字段相关的都留这里）    internal/locales
//	normalize.go  Normalize：缺项补齐、越界纠正、运行态覆盖       internal/netaddr
//	persist.go    Load / Save / runtime.* 回落                  本包
//	net.go        按默认路由挑网卡的 IP 探测                     internal/netif
//	path.go       DefaultPath 与用户配置目录                    os
//	extra.go      额外 DHCP 选项解析（option 43 的子项）        本包
//
// 新增配置项的顺序：本包 config.go 加字段 → normalize.go 补校验/回落 →
// internal/ui 对应页面加控件 → docs/ 同步说明。
package config
