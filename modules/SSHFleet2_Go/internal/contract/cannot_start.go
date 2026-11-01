package contract

// CannotStart 是「起不来」：前提缺了，连计划都生不出来。
//
// 它不是一种权力，是任何一站都可能撞上的事实，所以谁都能造——类型可见、没有构造函数。
// 它不实现 error 接口：那会在契约包里长一个 Error() 方法，与「契约只放类型与常量」打架；
// 塔尖按类型认它。
//
// 判据见 ADR-0034 决定三；它许在哪儿造，见 ADR-0032 第四条（闸门里造即报）。
type CannotStart struct {
	// Where 是老版错误屏上 [function:xxx] 那一格：撞上的是哪一环（config、cli、dangercheck…）。
	Where string

	// Say 是那一屏话：由撞上的那一处把事实交给 message 拼好，这里只搬。
	Say Text

	// Body 是**给日志看**的那一段：不带 [ERROR] 前缀、不带颜色，就是老三行那几句话。
	// 屏与记要形态不同——屏给使用者看（ADR-0028 第五节），日志留现场（第四节）。
	Body string
}
