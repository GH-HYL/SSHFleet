// Package credential 是横切叶子包：读主密钥、解密、报它齐不齐。
//
// 它只给「齐不齐」这个状态，不判断该不该跑——那是塔尖在解析递回信封之后问的一句。
//
// 判据见 ADR-0032 第四条。
package credential
