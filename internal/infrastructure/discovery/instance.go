package discovery

const (
	renewalIntervalSecs = 30
	leaseDurationSecs   = 90
)

type portInfo struct {
	Value   int  `json:"$"`
	Enabled bool `json:"@enabled"`
}

type dataCenterInfo struct {
	Class string `json:"@class"`
	Name  string `json:"name"`
}

type leaseInfo struct {
	RenewalIntervalInSecs int `json:"renewalIntervalInSecs"`
	DurationInSecs        int `json:"durationInSecs"`
}

type instanceInfo struct {
	InstanceID       string         `json:"instanceId"`
	HostName         string         `json:"hostName"`
	App              string         `json:"app"`
	IPAddr           string         `json:"ipAddr"`
	Status           string         `json:"status"`
	Port             portInfo       `json:"port"`
	SecurePort       portInfo       `json:"securePort"`
	VipAddress       string         `json:"vipAddress"`
	SecureVipAddress string         `json:"secureVipAddress"`
	HomePageURL      string         `json:"homePageUrl"`
	StatusPageURL    string         `json:"statusPageUrl"`
	HealthCheckURL   string         `json:"healthCheckUrl"`
	DataCenterInfo   dataCenterInfo `json:"dataCenterInfo"`
	LeaseInfo        leaseInfo      `json:"leaseInfo"`
}

type registrationPayload struct {
	Instance instanceInfo `json:"instance"`
}
