package main

/*
func BenchmarkOptimizedSerialization(b *testing.B) {

	BYTES := 1200

	// Creating Random Payload
	randomPayload := make([]byte, BYTES)
	rand.Read(randomPayload)

	// Preparing Serialization

	listenAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:31234")
	if err != nil {
		b.Fatal(err)
	}

	remoteAddr, err := appnet.ResolveUDPAddr("19-ffaa:1:eff,[127.0.0.1]:31234")
	if err != nil {
		b.Fatal(err)
	}

	packetSerializer, err := optimizedconn.NewPacketSerializer(
		addr.IA{
			I: 42,
			A: 42,
		},
		listenAddr,
		remoteAddr,
	)
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(BYTES))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err = packetSerializer.Serialize(randomPayload)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStandardSerialization(b *testing.B) {

	BYTES := 1200

	// Creating Random Payload
	randomPayload := make([]byte, BYTES)
	rand.Read(randomPayload)

	listenAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:31234")
	if err != nil {
		b.Fatal(err)
	}

	remoteAddr, err := appnet.ResolveUDPAddr("19-ffaa:1:eff,[127.0.0.1]:31234")
	if err != nil {
		b.Fatal(err)
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

	b.SetBytes(int64(BYTES))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pkt := &snet.Packet{
			Bytes: snet.Bytes(packetBuffer),
			PacketInfo: snet.PacketInfo{
				Destination: scionDestinationAddress,
				Source:      scionListenAddress,
				Path:        remoteAddr.Path,
				Payload: snet.UDPPayload{
					SrcPort: uint16(listenAddr.Port),
					DstPort: uint16(remoteAddr.Host.Port),
					Payload: randomPayload,
				},
			},
		}

		err = pkt.Serialize()
		if err != nil {
			b.Fatal(err)
		}
	}

}
*/
