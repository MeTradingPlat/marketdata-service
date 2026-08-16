package discovery

import (
	"fmt"
	"net"
)

// selfIPv4 busca la IP de la red bridge de Docker propia del contenedor --
// es la unica direccion que Eureka/Gateway pueden alcanzar de verdad desde
// otro contenedor. Se resuelve en caliente (no por config) porque cambia en
// cada arranque del contenedor.
func selfIPv4() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("listing network interfaces: %w", err)
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			return ip4.String(), nil
		}
	}
	return "", fmt.Errorf("no non-loopback ipv4 address found")
}
