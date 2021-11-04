package optimizedconn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/netsec-ethz/scion-apps/pkg/appnet"
	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/serrors"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/topology/underlay"
)

type MergedConn interface {
	net.Conn
	net.PacketConn
}

type OptimizedSCIONConn struct {
	readMtx sync.Mutex

	transportConn     MergedConn
	unixTransportConn MergedConn
	udpTransportConn  MergedConn

	listenAddr   *net.UDPAddr
	packetParser *PacketParser

	// Only populated, if user opened connection with Dial
	// Otherwise the connection does not support send functionality.
	remoteAddr       *snet.UDPAddr
	nextHop          *net.UDPAddr
	packetSerializer *PacketSerializer

	connectivityContext *ConnectivityContext
}

var _ net.Conn = &OptimizedSCIONConn{}

func Listen(listenAddr *net.UDPAddr) (*OptimizedSCIONConn, error) {

	if listenAddr == nil || listenAddr.IP == nil || listenAddr.IP.IsUnspecified() {
		return nil, serrors.New("listen addr is unspecified")
	}

	ctx := context.Background()
	connectivityContext, err := PrepareConnectivityContext(ctx)
	if err != nil {
		return nil, err
	}

	unixTransportPacketConn, assignedPort, err := connectivityContext.Dispatcher.Register(ctx, connectivityContext.LocalIA, listenAddr, addr.SvcNone)
	unixTransportConn := unixTransportPacketConn.(MergedConn)

	if err != nil {
		return nil, err
	}

	listenAddr.Port = int(assignedPort)

	ENABLE_FAST, hasFastEnv := os.LookupEnv("ENABLE_FAST")
	enableFast := hasFastEnv && ENABLE_FAST == "true"

	var udpTransportConn MergedConn

	if enableFast {
		udpTransportConn, err = net.ListenUDP("udp4", listenAddr)
		if err != nil {
			return nil, err
		}
	}

	var transportConn MergedConn

	if udpTransportConn != nil {
		fmt.Printf("Using udp as transportConn\n")
		transportConn = udpTransportConn
	} else {
		fmt.Printf("Using unix as transportConn\n")
		transportConn = unixTransportConn
	}

	packetParser, err := NewPacketParser()

	if err != nil {
		return nil, err
	}

	optimizedSCIONConn := OptimizedSCIONConn{
		transportConn:       transportConn,
		connectivityContext: connectivityContext,

		listenAddr: listenAddr,
		remoteAddr: nil,

		packetParser: packetParser,

		unixTransportConn: unixTransportConn,
		udpTransportConn:  udpTransportConn,
	}

	return &optimizedSCIONConn, nil
}

func Dial(listenAddr *net.UDPAddr, remoteAddr *snet.UDPAddr) (*OptimizedSCIONConn, error) {

	oSC, err := Listen(listenAddr)

	if err != nil {
		return nil, err
	}

	// We check, if there is a path.
	if remoteAddr.Path.IsEmpty() {

		err := appnet.SetDefaultPath(remoteAddr)
		if err != nil {
			return nil, err
		}
	}

	nextHop := remoteAddr.NextHop

	fmt.Printf("localIA=%v, remoteIA=%v\n", oSC.connectivityContext.LocalIA.String(), remoteAddr.IA.String())
	if nextHop == nil && oSC.connectivityContext.LocalIA.Equal(remoteAddr.IA) {
		if bytes.Compare(remoteAddr.Host.IP, oSC.listenAddr.IP) == 0 {
			nextHop = &net.UDPAddr{
				IP:   remoteAddr.Host.IP,
				Port: remoteAddr.Host.Port,
				Zone: remoteAddr.Host.Zone,
			}
		} else {
			nextHop = &net.UDPAddr{
				IP:   remoteAddr.Host.IP,
				Port: underlay.EndhostPort,
				Zone: remoteAddr.Host.Zone,
			}
		}

	}

	fmt.Printf("Using nextHop=%v\n", nextHop)

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

	oSC.packetSerializer = packetSerializer

	return oSC, nil

}

func (oSC *OptimizedSCIONConn) SetRemote(remoteAddr *snet.UDPAddr) error {
	// We check, if there is a path.
	if remoteAddr.Path.IsEmpty() {

		err := appnet.SetDefaultPath(remoteAddr)
		if err != nil {
			return err
		}
	}

	nextHop := remoteAddr.NextHop

	fmt.Printf("localIA=%v, remoteIA=%v\n", oSC.connectivityContext.LocalIA.String(), remoteAddr.IA.String())
	if nextHop == nil && oSC.connectivityContext.LocalIA.Equal(remoteAddr.IA) {
		if bytes.Compare(remoteAddr.Host.IP, oSC.listenAddr.IP) == 0 {
			nextHop = &net.UDPAddr{
				IP:   remoteAddr.Host.IP,
				Port: remoteAddr.Host.Port,
				Zone: remoteAddr.Host.Zone,
			}
		} else {
			nextHop = &net.UDPAddr{
				IP:   remoteAddr.Host.IP,
				Port: underlay.EndhostPort,
				Zone: remoteAddr.Host.Zone,
			}
		}
	}

	fmt.Printf("Using nextHop=%v\n", nextHop)

	oSC.remoteAddr = remoteAddr
	oSC.nextHop = nextHop

	packetSerializer, err := NewPacketSerializer(
		oSC.connectivityContext.LocalIA,
		oSC.listenAddr,
		remoteAddr,
	)

	if err != nil {
		return err
	}

	oSC.packetSerializer = packetSerializer
	return nil
}

func (c *OptimizedSCIONConn) Close() error {

	if c.udpTransportConn != nil {
		err := c.udpTransportConn.Close()

		if err != nil {
			return err
		}
	}

	return c.unixTransportConn.Close()
}

func (c *OptimizedSCIONConn) Read(b []byte) (int, error) {

	n, err := c.transportConn.Read(c.packetParser.ReadBuffer)

	if err != nil {
		return 0, err
	}

	payloadLen, err := c.packetParser.Parse(n, b)

	if err != nil {
		return 0, err
	}

	return payloadLen, nil
}

func (c *OptimizedSCIONConn) Write(b []byte) (int, error) {

	if c.nextHop == nil || c.remoteAddr == nil || c.packetSerializer == nil {
		return 0, errors.New("Connection does not support send functionality")
	}

	buffer, err := c.packetSerializer.Serialize(b)
	if err != nil {
		return 0, err
	}

	_, err = c.transportConn.WriteTo(buffer, c.nextHop)

	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *OptimizedSCIONConn) LocalAddr() net.Addr {
	return c.listenAddr
}

func (c *OptimizedSCIONConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *OptimizedSCIONConn) SetDeadline(t time.Time) error {
	if c.udpTransportConn != nil {
		err := c.udpTransportConn.SetDeadline(t)

		if err != nil {
			return err
		}
	}

	return c.unixTransportConn.SetDeadline(t)
}

func (c *OptimizedSCIONConn) SetReadDeadline(t time.Time) error {
	if c.udpTransportConn != nil {
		err := c.udpTransportConn.SetReadDeadline(t)

		if err != nil {
			return err
		}
	}

	return c.unixTransportConn.SetReadDeadline(t)
}

func (c *OptimizedSCIONConn) SetWriteDeadline(t time.Time) error {
	if c.udpTransportConn != nil {
		err := c.udpTransportConn.SetWriteDeadline(t)

		if err != nil {
			return err
		}
	}

	return c.unixTransportConn.SetWriteDeadline(t)
}
