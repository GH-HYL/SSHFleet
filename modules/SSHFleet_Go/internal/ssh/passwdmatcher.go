package ssh

// 改密的步骤匹配器。
//
// 与代填的游标匹配器**共用**「在游标之后找最先出现的关键词」这一件（`keywordRule`），
// 消费语义**相反**：代填一条规则只送一次（命中即 done），改密每条可送 `max` 次——
// 弱新密码会被 PAM 拒并重问，同一个提示要能再答一次。
//
// 另有一处代填没有的东西：**入场信号**。没出现过期信号之前，各条提示一律不参战——
// 没过期的机器上就不会有这些提示，做门是白拿的一层保险（实测 M2/M4：警告行必然先于
// 「当前的密码」出现）。

// passwdHit 一次命中。
type passwdHit struct {
	enter bool // 命中入场信号
	step  int  // 命中的步骤序号（0 基；enter 为真时无意义）
	end   int  // 命中终点：游标推进到此处
}

// passwdRule 一条可重复消费的规则。
type passwdRule struct {
	rule  keywordRule
	value PasswdValue
	left  int // 还能喂几次
}

// passwdMatcher 步骤匹配器。非并发安全：调用方在 outputTap 的锁里用。
type passwdMatcher struct {
	enter    keywordRule
	sawEnter bool
	rules    []passwdRule
	opts     MatchOptions
	tail     string // 游标之后的字节：匹配范围
}

// newPasswdMatcher 按提示词表与入场信号关键词建匹配器。
// enterKeywords 为空时入场信号永不命中——调用方应在启动阶段就拦住这种配置（见 main）。
func newPasswdMatcher(prompts *PasswdPrompts, enterKeywords []string, opts MatchOptions) *passwdMatcher {
	m := &passwdMatcher{
		enter: newKeywordRule(enterKeywords, opts),
		opts:  opts,
	}
	for _, step := range prompts.Steps {
		m.rules = append(m.rules, passwdRule{
			rule:  newKeywordRule(step.Keywords, opts),
			value: step.Value,
			left:  step.Max,
		})
	}
	return m
}

// feed 追加新到的字节，返回本次该喂的值（按出现先后）以及「无话可给」这件事。
//
// 第二个返回值：**某条已经喂满的提示又出现了**——它在问，而我们给不出新的答案。
// 调用方应当立刻收场，不干等 `-t`（2026-09-29 实测：新密码被目标机的密码规则反复拒掉时，
// 远端会一直重问，工具白等到 `-t` 到点）。
func (m *passwdMatcher) feed(chunk string) (values []PasswdValue, stuck bool) {
	m.tail += chunk

	for {
		hit, ok := m.next()
		if !ok {
			break
		}
		m.tail = m.tail[hit.end:] // 游标推进到命中终点（不是段末：一块里可能连着两处提示）
		if hit.enter {
			m.sawEnter = true
			continue
		}
		m.rules[hit.step].left--
		values = append(values, m.rules[hit.step].value)
	}
	// 顺序要紧：先判「又问了一遍」，再清尾部——清了就看不见它了
	stuck = m.askedAgain()
	if m.exhausted() {
		m.tail = "" // 全表喂满：尾部与游标都不再需要（输出本身已由采集缓冲完整留存）
	}
	return values, stuck
}

// askedAgain 游标之后是否出现了已经喂满的那几条提示。
func (m *passwdMatcher) askedAgain() bool {
	for i := range m.rules {
		if m.rules[i].left > 0 {
			continue
		}
		if _, _, _, ok := m.rules[i].rule.find(m.tail, m.opts); ok {
			return true
		}
	}
	return false
}

// next 在游标之后找最先出现的一处命中。
//
// 两态互斥：入场信号还没出现时**只有它参与**——各条提示一律不参战（未过期的机器上本来就
// 不会有这些提示，做门是白拿的一层保险）；它出现之后轮到还没喂满的步骤，取位置最靠前者。
func (m *passwdMatcher) next() (passwdHit, bool) {
	if !m.sawEnter {
		if _, end, _, ok := m.enter.find(m.tail, m.opts); ok {
			return passwdHit{enter: true, end: end}, true
		}
		return passwdHit{}, false
	}

	best := passwdHit{step: -1}
	bestStart := -1
	for i := range m.rules {
		if m.rules[i].left <= 0 {
			continue
		}
		start, end, _, ok := m.rules[i].rule.find(m.tail, m.opts)
		if !ok || (bestStart >= 0 && start >= bestStart) {
			continue
		}
		best, bestStart = passwdHit{step: i, end: end}, start
	}
	if bestStart < 0 {
		return passwdHit{}, false
	}
	return best, true
}

// exhausted 全表都喂满了。
func (m *passwdMatcher) exhausted() bool {
	for _, r := range m.rules {
		if r.left > 0 {
			return false
		}
	}
	return true
}

// sawEnterSignal 整场是否出现过入场信号——收尾判定用：没出现 = 这台机器不需要改密。
func (m *passwdMatcher) sawEnterSignal() bool { return m.sawEnter }
