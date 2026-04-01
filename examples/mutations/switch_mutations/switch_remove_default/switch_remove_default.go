package switch_remove_default

func GetDayType(day int) string {
	switch day {
	case 1:
		return "Monday"
	case 2:
		return "Tuesday"
	case 3:
		return "Wednesday"
	case 4:
		return "Thursday"
	case 5:
		return "Friday"
	case 6:
		return "Saturday"
	case 7:
		return "Sunday"
	default:
		return "Invalid"
	}
}

func GetGrade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}

func ProcessValue(val interface{}) string {
	switch val.(type) {
	case int:
		return "integer"
	case string:
		return "string"
	case float64:
		return "float"
	default:
		return "unknown"
	}
}
