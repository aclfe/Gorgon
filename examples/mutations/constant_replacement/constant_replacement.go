package constant_replacement

func GetMaxRetries() int {
	return 3
}

func GetDefaultPort() int {
	return 8080
}

func GetEmptyString() string {
	return ""
}

func GetDefaultScale() float64 {
	return 1.5
}

func GetMarker() byte {
	return 'x'
}

const MaxConnections = 100

func GetMaxConnections() int {
	return MaxConnections
}
