package negate_condition

func IsValid(age int) bool {
	return age >= 18
}

func HasAccess(isAdmin bool, hasPermission bool) bool {
	return isAdmin || hasPermission
}

func CheckBalance(amount, limit int) bool {
	return amount <= limit
}

func alreadyNegated(x bool) bool {
	return !x
}
