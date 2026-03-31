package logical_operator

func IsAdult(age int, hasLicense bool) bool {
	return age >= 18 && hasLicense
}

func CanVote(age int, isCitizen bool) bool {
	return age >= 18 && isCitizen
}

func HasDiscount(isMember bool, orderAbove100 bool) bool {
	return isMember || orderAbove100
}

func IsValid(isEmailVerified bool, hasPaymentMethod bool) bool {
	return isEmailVerified || hasPaymentMethod
}
