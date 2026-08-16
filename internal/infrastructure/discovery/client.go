package discovery

import (
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	appName    string
	instanceID string
	instance   instanceInfo
	httpClient *http.Client
}

// NewClient arma el payload de registro una sola vez con la IP propia del
// contenedor -- Register/Heartbeat/Deregister solo la reenvian mientras el
// proceso viva.
func NewClient(baseURL, appName, vipAddress string, port int) (*Client, error) {
	ip, err := selfIPv4()
	if err != nil {
		return nil, fmt.Errorf("resolving self ip for eureka registration: %w", err)
	}

	instanceID := fmt.Sprintf("%s:%s:%d", ip, vipAddress, port)
	base := fmt.Sprintf("http://%s:%d", ip, port)

	return &Client{
		baseURL:    baseURL,
		appName:    appName,
		instanceID: instanceID,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		instance: instanceInfo{
			InstanceID:       instanceID,
			HostName:         ip,
			App:              appName,
			IPAddr:           ip,
			Status:           "UP",
			Port:             portInfo{Value: port, Enabled: true},
			SecurePort:       portInfo{Value: 443, Enabled: false},
			VipAddress:       vipAddress,
			SecureVipAddress: vipAddress,
			HomePageURL:      base + "/",
			StatusPageURL:    base + "/marketdata/health",
			HealthCheckURL:   base + "/marketdata/health",
			DataCenterInfo:   dataCenterInfo{Class: "com.netflix.appinfo.InstanceInfo$DefaultDataCenterInfo", Name: "MyOwn"},
			LeaseInfo:        leaseInfo{RenewalIntervalInSecs: renewalIntervalSecs, DurationInSecs: leaseDurationSecs},
		},
	}, nil
}
