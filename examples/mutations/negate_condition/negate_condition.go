package negate_condition

func GetStatus(score int) string {
	if score >= 60 {
		//gorgon:ignore
		return "pass"
	}
	return "fail"
}

func CheckAccess(level, required int) bool {
	if level >= required {
		return true
	}
	return false
}
