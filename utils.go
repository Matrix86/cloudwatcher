package cloudwatcher

func inArray(needle interface{}, generic interface{}) bool {
	if haystack, ok := generic.([]string); ok {
		for _, v := range haystack {
			if v == needle {
				return true
			}
		}
	}
	return false
}
