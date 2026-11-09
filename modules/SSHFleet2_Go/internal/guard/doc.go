// Package guard 是结构检查包：一张包清单规则表，加跟着 go test 跑的越界检查。
//
// 红线区与它同住，不塞进任何一站——它要把全工程摊开数。
//
// 它看得见四种拆法：删入口（`registry.go` 的 `EntryFiles`）、删测试（`redline.go` 的红线名单）、
// 掏空测试体里的断言、掏空 helper（后两样同住 `redline.go`）。判据见 ADR-0045 与 ADR-0046。
//
// 判据见 ADR-0032 第四条「怎么核」与 ADR-0037。
package guard
