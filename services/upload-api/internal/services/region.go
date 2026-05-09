package services

var countryToRegionMap = map[string]string{
	"US": "us-central1", "CA": "us-central1", "MX": "us-central1",
	"BR": "us-central1", "AR": "us-central1", "CL": "us-central1",

	"GB": "europe-west1", "FR": "europe-west1", "DE": "europe-west1",
	"ES": "europe-west1", "IT": "europe-west1", "NL": "europe-west1",
	"SE": "europe-west1", "NO": "europe-west1", "PL": "europe-west1",

	"ZA": "europe-west1", "EG": "europe-west1", "NG": "europe-west1",

	"JP": "asia-east1", "CN": "asia-east1", "KR": "asia-east1",
	"IN": "asia-east1", "SG": "asia-east1", "TW": "asia-east1",

	"AU": "asia-east1", "NZ": "asia-east1",
}

const defaultRegion = "asia-south1"

func (s *uploadService) resolveRegion(countryCode string) string {
	if region, exists := countryToRegionMap[countryCode]; exists {
		return region
	}
	return defaultRegion
}
