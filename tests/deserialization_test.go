package main

import (
	"math/rand"
	"net"
	"testing"

	"github.com/netsec-ethz/scion-apps/pkg/appnet"
	optimizedconn "github.com/netsys-lab/scion-optimized-connection/pkg"
	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/common"
	"github.com/scionproto/scion/go/lib/snet"
)

func PreparePacket(payload []byte) (*snet.Packet, error) {
	listenAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:31234")
	if err != nil {
		return nil, err
	}

	remoteAddr, err := appnet.ResolveUDPAddr("19-ffaa:1:eff,[127.0.0.1]:31234")
	if err != nil {
		return nil, err
	}

	scionDestinationAddress := snet.SCIONAddress{
		IA:   remoteAddr.IA,
		Host: addr.HostFromIP(remoteAddr.Host.IP),
	}

	scionListenAddress := snet.SCIONAddress{
		IA: addr.IA{
			I: 42,
			A: 42,
		},
		Host: addr.HostFromIP(listenAddr.IP),
	}

	packetBuffer := make([]byte, common.MaxMTU)

	pkt := &snet.Packet{
		Bytes: snet.Bytes(packetBuffer),
		PacketInfo: snet.PacketInfo{
			Destination: scionDestinationAddress,
			Source:      scionListenAddress,
			Path:        remoteAddr.Path,
			Payload: snet.UDPPayload{
				SrcPort: uint16(listenAddr.Port),
				DstPort: uint16(remoteAddr.Host.Port),
				Payload: payload,
			},
		},
	}

	err = pkt.Serialize()

	if err != nil {
		return nil, err
	}

	return pkt, nil

}

func BenchmarkOptimizedDeserialization(b *testing.B) {

	BYTES := 1200

	// Creating Random Payload
	randomPayload := make([]byte, BYTES)
	rand.Read(randomPayload)

	pkt, err := PreparePacket(randomPayload)
	if err != nil {
		b.Fatal(err)
	}

	packetParser, err := optimizedconn.NewPacketParser()
	if err != nil {
		b.Fatal(err)
	}

	copy(packetParser.ReadBuffer, pkt.Bytes)

	readBuffer := make([]byte, common.MaxMTU)

	b.ResetTimer()
	b.SetBytes(int64(BYTES))

	for i := 0; i < b.N; i++ {
		_, err = packetParser.Parse(len(pkt.Bytes), readBuffer)

		if err != nil {
			b.Fatal(err)
		}
	}

}

func BenchmarkStandardDeserialization(b *testing.B) {

	BYTES := 1200

	// Creating Random Payload
	randomPayload := make([]byte, BYTES)
	rand.Read(randomPayload)

	pkt, err := PreparePacket(randomPayload)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.SetBytes(int64(BYTES))

	byteBuffer := make([]byte, common.MaxMTU)

	for i := 0; i < b.N; i++ {

		copy(byteBuffer, pkt.Bytes)

		decodePkt := snet.Packet{
			Bytes: byteBuffer,
		}

		err = decodePkt.Decode()

		if err != nil {
			b.Fatal(err)
		}

	}

}
