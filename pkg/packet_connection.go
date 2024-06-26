package optimizedconn

import (
	"bytes"
	"context"
	"net"
	"sync"
	"time"

	"github.com/scionproto/scion/pkg/private/serrors"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/private/topology"
)

type OptimizedSCIONPacketConn struct {
	readMtx sync.Mutex

	transportConn     MergedConn
	unixTransportConn MergedConn
	udpTransportConn  MergedConn

	listenAddr   *net.UDPAddr
	packetParser *PacketParser

	// Only populated, if user opened connection with Dial
	// Otherwise the connection does not support send functionality.
	remoteAddr        *snet.UDPAddr
	nextHop           *net.UDPAddr
	packetSerializers map[string]*PacketSerializer

	connectivityContext *ConnectivityContext
}

var _ net.Conn = &OptimizedSCIONConn{}

func ListenPacket(listenAddr *net.UDPAddr) (*OptimizedSCIONPacketConn, error) {

	if listenAddr == nil || listenAddr.IP == nil || listenAddr.IP.IsUnspecified() {
		return nil, serrors.New("listen addr is unspecified")
	}

	ctx := context.Background()
	connectivityContext, err := PrepareConnectivityContext(ctx)
	if err != nil {
		return nil, err
	}

	//unixTransportPacketConn, assignedPort, err := connectivityContext.Dispatcher.Register(ctx, connectivityContext.LocalIA, listenAddr, addr.SvcNone)
	//unixTransportConn := unixTransportPacketConn.(MergedConn)

	//if err != nil {
	//	return nil, err
	//}

	// listenAddr.Port = int(assignedPort)

	//ENABLE_FAST, hasFastEnv := os.LookupEnv("ENABLE_FAST")
	//enableFast := hasFastEnv && ENABLE_FAST == "true"

	var udpTransportConn MergedConn

	// if enableFast {
	udpTransportConn, err = net.ListenUDP("udp4", listenAddr)
	if err != nil {
		return nil, err
	}
	//}

	/*var transportConn MergedConn

	if udpTransportConn != nil {
		//fmt.Printf("Using udp as transportConn\n")
		transportConn = udpTransportConn
	} else {
		//fmt.Printf("Using unix as transportConn\n")
		transportConn = unixTransportConn
	}*/

	packetParser, err := NewPacketParser()

	if err != nil {
		return nil, err
	}

	optimizedSCIONConn := OptimizedSCIONPacketConn{
		transportConn:       udpTransportConn,
		connectivityContext: connectivityContext,

		listenAddr: listenAddr,
		remoteAddr: nil,

		packetParser: packetParser,

		// unixTransportConn: unixTransportConn,
		udpTransportConn:  udpTransportConn,
		packetSerializers: make(map[string]*PacketSerializer),
	}

	return &optimizedSCIONConn, nil
}

// TODO: This needs to be fixed...
func PathToString(path snet.Path) string {
	// iterate over path.Metadata().Interfaces and append a string of all interfaces to a string
	// return the string

	if path == nil {
		return ""
	}

	if path.Metadata() == nil {
		// fmt.Println("Path metadata is nil")
		// Random number
		// return string(rand.Intn(100))
		return ""
	}

	var pathString string
	for _, intf := range path.Metadata().Interfaces {
		pathString += intf.String()
	}
	return pathString
}

func (oSC *OptimizedSCIONPacketConn) addRemote(remoteAddr *snet.UDPAddr) (*PacketSerializer, error) {

	if remoteAddr.Path == nil {

		// err := appnet.SetDefaultPath(remoteAddr)
		// log.Fatal("No path found")
		err := setDefaultPath(oSC.connectivityContext.DaemonConn, context.Background(), remoteAddr)
		if err != nil {
			return nil, err
		}
	}

	path, err := remoteAddr.GetPath()
	if err != nil {
		return nil, err
	}

	//fmt.Println("Got path ")
	//fmt.Println(path)

	ps, ok := oSC.packetSerializers[remoteAddr.String()+"-"+PathToString(path)]
	if !ok {
		// We check, if there is a path.

		nextHop := remoteAddr.NextHop

		// fmt.Printf("localIA=%v, remoteIA=%v\n", oSC.connectivityContext.LocalIA.String(), remoteAddr.IA.String())
		if nextHop == nil && oSC.connectivityContext.LocalIA.Equal(remoteAddr.IA) {
			if bytes.Equal(remoteAddr.Host.IP, oSC.listenAddr.IP) {
				nextHop = &net.UDPAddr{
					IP:   remoteAddr.Host.IP,
					Port: remoteAddr.Host.Port,
					Zone: remoteAddr.Host.Zone,
				}
			} else {
				nextHop = &net.UDPAddr{
					IP:   remoteAddr.Host.IP,
					Port: topology.EndhostPort,
					Zone: remoteAddr.Host.Zone,
				}
			}
		}

		// fmt.Printf("Using nextHop=%v\n", nextHop)

		oSC.remoteAddr = remoteAddr
		oSC.nextHop = nextHop

		packetSerializer, err := NewPacketSerializer(
			oSC.connectivityContext.LocalIA,
			oSC.listenAddr,
			remoteAddr,
		)

		if err != nil {
			return nil, err
		}

		oSC.packetSerializers[remoteAddr.String()+"-"+PathToString(path)] = packetSerializer
		return packetSerializer, nil
	}

	return ps, nil
}

func (c *OptimizedSCIONPacketConn) Close() error {

	if c.udpTransportConn != nil {
		err := c.udpTransportConn.Close()

		if err != nil {
			return err
		}
	}

	return c.unixTransportConn.Close()
}

func (c *OptimizedSCIONPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {

	n, addr, err := c.transportConn.ReadFrom(c.packetParser.ReadBuffer)

	if err != nil {
		return 0, nil, err
	}

	payloadLen, err := c.packetParser.Parse(n, b)

	if err != nil {
		return 0, nil, err
	}

	return payloadLen, addr, nil
}

func (c *OptimizedSCIONPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {

	sAddr, ok := addr.(*snet.UDPAddr)
	if !ok {
		return 0, serrors.New("addr is not of type *snet.UDPAddr")
	}
	serializer, err := c.addRemote(sAddr)
	if err != nil {
		return 0, err
	}
	// serializer := c.packetSerializers[sAddr.String()]

	buffer, err := serializer.Serialize(b)
	if err != nil {
		return 0, err
	}

	nextHop := c.getNextHop(sAddr)

	_, err = c.transportConn.WriteTo(buffer, nextHop)

	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (oSC *OptimizedSCIONPacketConn) getNextHop(remoteAddr *snet.UDPAddr) *net.UDPAddr {
	nextHop := remoteAddr.NextHop

	//fmt.Printf("localIA=%v, remoteIA=%v\n", oSC.connectivityContext.LocalIA.String(), remoteAddr.IA.String())
	//fmt.Printf("Destination: %s\n", remoteAddr.String())
	// fmt.Println(nextHop)
	if nextHop == nil && oSC.connectivityContext.LocalIA.Equal(remoteAddr.IA) {
		if bytes.Equal(remoteAddr.Host.IP, oSC.listenAddr.IP) {
			nextHop = &net.UDPAddr{
				IP:   remoteAddr.Host.IP,
				Port: topology.EndhostPort,
				Zone: remoteAddr.Host.Zone,
			}

			if oSC.udpTransportConn != nil {
				nextHop = &net.UDPAddr{
					IP:   remoteAddr.Host.IP,
					Port: remoteAddr.Host.Port,
					Zone: remoteAddr.Host.Zone,
				}
			}

		} else {
			nextHop = &net.UDPAddr{
				IP:   remoteAddr.Host.IP,
				Port: topology.EndhostPort,
				Zone: remoteAddr.Host.Zone,
			}

		}
	}

	//fmt.Printf("Using nextHop=%v\n", nextHop)
	return nextHop
}

func (c *OptimizedSCIONPacketConn) LocalAddr() net.Addr {
	return c.listenAddr
}

func (c *OptimizedSCIONPacketConn) SetDeadline(t time.Time) error {
	if c.udpTransportConn != nil {
		err := c.udpTransportConn.SetDeadline(t)

		if err != nil {
			return err
		}
	}

	return c.unixTransportConn.SetDeadline(t)
}

func (c *OptimizedSCIONPacketConn) SetReadDeadline(t time.Time) error {
	if c.udpTransportConn != nil {
		err := c.udpTransportConn.SetReadDeadline(t)

		if err != nil {
			return err
		}
	}

	return c.unixTransportConn.SetReadDeadline(t)
}

func (c *OptimizedSCIONPacketConn) SetWriteDeadline(t time.Time) error {
	if c.udpTransportConn != nil {
		err := c.udpTransportConn.SetWriteDeadline(t)

		if err != nil {
			return err
		}
	}

	return c.unixTransportConn.SetWriteDeadline(t)
}
