// Copyright 2015 CoreOS, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package updateengine

import (
	"fmt"
	"os"
	"strconv"

	"github.com/godbus/dbus"
)

const (
	dbusPath            = "/com/coreos/update1"
	dbusInterface       = "com.coreos.update1.Manager"
	dbusMember          = "StatusUpdate"
	dbusMemberInterface = dbusInterface + "." + dbusMember
	signalBuffer        = 32 // TODO(bp): What is a reasonable value here?
)

type Client struct {
	conn   *dbus.Conn
	object dbus.BusObject
	ch     chan *dbus.Signal
}

func New() (*Client, error) {
	c := new(Client)
	var err error

	c.conn, err = dbus.SystemBusPrivate()
	if err != nil {
		return nil, err
	}

	methods := []dbus.Auth{dbus.AuthExternal(strconv.Itoa(os.Getuid()))}
	err = c.conn.Auth(methods)
	if err != nil {
		c.conn.Close()
		return nil, err
	}

	err = c.conn.Hello()
	if err != nil {
		c.conn.Close()
		return nil, err
	}

	c.object = c.conn.Object("com.coreos.update1", dbus.ObjectPath(dbusPath))

	// Setup the filter for the StatusUpdate signals
	match := fmt.Sprintf("type='signal',interface='%s',member='%s'", dbusInterface, dbusMember)

	call := c.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, match)
	if call.Err != nil {
		return nil, call.Err
	}

	c.ch = make(chan *dbus.Signal, signalBuffer)
	c.conn.Signal(c.ch)

	return c, nil
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ReceiveStatuses receives signal messages from dbus and sends them as Statues
// on the rcvr channel, until the stop channel is closed. An attempt is made to
// get the initial status and send it on the rcvr channel before receiving
// starts.
func (c *Client) ReceiveStatuses(rcvr chan Status, stop <-chan struct{}) {
	// if there is an error getting the current status, ignore it and just
	// move onto the main loop.
	st, _ := c.GetStatus()
	rcvr <- st

	for {
		select {
		case <-stop:
			return
		case signal := <-c.ch:
			rcvr <- NewStatus(signal.Body)
		}
	}
}

func (c *Client) RebootNeededSignal(rcvr chan Status, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case signal := <-c.ch:
			s := NewStatus(signal.Body)
			if s.CurrentOperation == UpdateStatusUpdatedNeedReboot {
				rcvr <- s
			}
		}
	}
}

// GetStatus gets the current status from update_engine
func (c *Client) GetStatus() (Status, error) {
	call := c.object.Call(dbusInterface+".GetStatus", 0)
	if call.Err != nil {
		return Status{}, call.Err
	}
	return NewStatus(call.Body), nil
}

// AttemptUpdate will trigger an update if available. This is an asynchronous
// call - it returns immediately.
func (c *Client) AttemptUpdate() error {
	call := c.object.Call(dbusInterface+".AttemptUpdate", 0)
	return call.Err
}
