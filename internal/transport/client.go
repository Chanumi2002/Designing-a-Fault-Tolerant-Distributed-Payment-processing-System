package transport

import "net"

// UDPClient sends UDP messages to other nodes
type UDPClient struct{}

func NewUDPClient() *UDPClient {
	return &UDPClient{}
}

// Send sends raw bytes to a peer address
func (c *UDPClient) Send(addr string, data []byte) error {
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write(data)
	return err
}
