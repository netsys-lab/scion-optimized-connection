package optimizedconn

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/scionproto/scion/pkg/private/serrors"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/private/topology"
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

	var udpTransportConn MergedConn

	udpTransportConn, err = net.ListenUDP("udp4", listenAddr)
	if err != nil {
		return nil, err
	}

	packetParser, err := NewPacketParser()

	if err != nil {
		return nil, err
	}

	optimizedSCIONConn := OptimizedSCIONConn{
		transportConn:       udpTransportConn,
		connectivityContext: connectivityContext,

		listenAddr: listenAddr,
		remoteAddr: nil,

		packetParser: packetParser,

		udpTransportConn: udpTransportConn,
	}

	return &optimizedSCIONConn, nil
}

func Dial(listenAddr *net.UDPAddr, remoteAddr *snet.UDPAddr) (*OptimizedSCIONConn, error) {

	oSC, err := Listen(listenAddr)

	if err != nil {
		return nil, err
	}

	nextHop := remoteAddr.NextHop

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
				Port: topology.EndhostPort,
				Zone: remoteAddr.Host.Zone,
			}
		}

	}

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

	nextHop := remoteAddr.NextHop

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
				Port: topology.EndhostPort,
				Zone: remoteAddr.Host.Zone,
			}
		}
	}

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
