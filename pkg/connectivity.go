package optimizedconn

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/pkg/sock/reliable"
)

// Took from appnet and modified a bit.

type ConnectivityContext struct {
	DaemonConn daemon.Connector
	Dispatcher reliable.Dispatcher
	LocalIA    addr.IA
}

func PrepareConnectivityContext(ctx context.Context) (*ConnectivityContext, error) {

	daemonConn, err := findSciond(ctx)
	if err != nil {
		return nil, err
	}

	dispatcher, err := findDispatcher()
	if err != nil {
		return nil, err
	}

	localIA, err := daemonConn.LocalIA(ctx)
	if err != nil {
		return nil, err
	}

	cContext := ConnectivityContext{
		DaemonConn: daemonConn,
		Dispatcher: dispatcher,
		LocalIA:    localIA,
	}

	return &cContext, nil

}

// Parts of this file were took from the scion-apps repository.
// Since they are not exposed on a regulary basis, we copied it into here to be able to use it.
// https://github.com/netsec-ethz/scion-apps/blob/master/pkg/appnet/appnet.go

// Copyright 2020 ETH Zurich
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

func findSciond(ctx context.Context) (daemon.Connector, error) {
	address, ok := os.LookupEnv("SCION_DAEMON_ADDRESS")
	if !ok {
		address = daemon.DefaultAPIAddress
	}
	sciondConn, err := daemon.NewService(address).Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to SCIOND at %s (override with SCION_DAEMON_ADDRESS): %w", address, err)
	}
	return sciondConn, nil
}

func findDispatcher() (reliable.Dispatcher, error) {
	path, err := findDispatcherSocket()
	if err != nil {
		return nil, err
	}
	dispatcher := reliable.NewDispatcher(path)
	return dispatcher, nil
}

func findDispatcherSocket() (string, error) {
	path, ok := os.LookupEnv("SCION_DISPATCHER_SOCKET")
	if !ok {
		path = reliable.DefaultDispPath
	}
	err := statSocket(path)
	if err != nil {
		return "", fmt.Errorf("error looking for SCION dispatcher socket at %s (override with SCION_DISPATCHER_SOCKET): %w", path, err)
	}
	return path, nil
}

func statSocket(path string) error {
	fileinfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !isSocket(fileinfo.Mode()) {
		return fmt.Errorf("%s is not a socket (mode: %s)", path, fileinfo.Mode())
	}
	return nil
}

func isSocket(mode os.FileMode) bool {
	return mode&os.ModeSocket != 0
}

func queryPaths(sciond daemon.Connector, ctx context.Context, dst *snet.UDPAddr) ([]snet.Path, error) {
	flags := daemon.PathReqFlags{Refresh: false, Hidden: false}
	snetPaths, err := sciond.Paths(ctx, dst.IA, 0, flags)
	return snetPaths, err
}

func setDefaultPath(sciond daemon.Connector, ctx context.Context, dst *snet.UDPAddr) error {
	paths, err := queryPaths(sciond, ctx, dst)
	if err != nil {
		return err
	}
	if len(paths) > 0 {
		dst.Path = paths[0].Dataplane()
		dst.NextHop = paths[0].UnderlayNextHop()
		return nil
	}

	return errors.New("No path found")

}
