// Package ask 是横切叶子包：摆出问题、收一行回答。
//
// 它不判断答案的含义——问完放不放行是闸门的事。叶子之间只有这一处依赖：→ console、→ message。
//
// 判据见 ADR-0032 第四条。
package ask
