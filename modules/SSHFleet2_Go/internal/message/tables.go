package message

// 判据表那几份文件的「出处」话（措辞迁移 03 号单）。
//
// 判据表写坏时屏上给的那一句「出处：」，说的是这一项该怎么写。它是给使用者看的——
// 改表的人照着这一句改，所以字住这里，不由 `parse` 拼。

// DangerRuleKnownFields 是危险规则表里出现不认识的字时，「出处：」那一行要说的话。
//
// 那六项与 `parse/tables.go` 的 `unknownRuleKeys` 是同一份判据的两面：那边拿它判、这边拿它
// 说。加一项、改一项，两处一起改——与 `parse/config.go` 顶部那两份路径同一条约定。
func DangerRuleKnownFields() string {
	return "规则只认 name / example / reason / regex / risk_level / enabled 六项，" +
		"把它删掉或改成这六项之一"
}
