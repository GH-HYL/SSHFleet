// 违规样本：工程里有汇编文件本身就报——它能 CALL 任意符号，不经 import。
TEXT ·runCommand(SB),NOSPLIT,$0-0
	RET
