package transport

import (
	"log"
	"net"

	"distributed_payment_system/internal/types"
)

// MessageHandler is called for every incoming parsed message
type MessageHandler func(msg *types.Message)

// UDPServer listens for incoming UDP messages
type UDPServer struct {
	Address string
	Handler MessageHandler
}

// NewUDPServer creates a new UDP server
func NewUDPServer(address string, handler MessageHandler) *UDPServer {
	return &UDPServer{
		Address: address,
		Handler: handler,
	}
}

// Start begins listening for incoming UDP packets
func (s *UDPServer) Start() error {
	addr, err := net.ResolveUDPAddr("udp", s.Address)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	log.Printf("UDP server listening on %s\n", s.Address)

	go func() {
		defer conn.Close()

		buffer := make([]byte, 65535)

		for {
			n, _, err := conn.ReadFromUDP(buffer)
			if err != nil {
				log.Println("udp read error:", err)
				continue
			}

			msg, err := types.ParseMessage(buffer[:n])
			if err != nil {
				log.Println("message parse error:", err)
				continue
			}

			if s.Handler != nil {
				s.Handler(msg)
			}
		}
	}()

	return nil
}