package hook

import (
	"fmt"
	"os"
	"strconv"

	"github.com/godbus/dbus"
)

const (
	dest  = "org.freedesktop.systemd1"
	iface = dest + ".Manager"
	opath = "/org/freedesktop/systemd1"
)

type Hook struct {
	conn   *dbus.Conn
	object dbus.BusObject
	unit   string
}

func New(unit string) (*Hook, error) {
	h := new(Hook)
	var err error

	h.unit = unit

	h.conn, err = dbus.SystemBusPrivate()
	if err != nil {
		return nil, err
	}

	methods := []dbus.Auth{dbus.AuthExternal(strconv.Itoa(os.Getuid()))}
	err = h.conn.Auth(methods)
	if err != nil {
		h.conn.Close()
		return nil, err
	}

	err = h.conn.Hello()
	if err != nil {
		h.conn.Close()
		return nil, err
	}

	h.object = h.conn.Object(dest, dbus.ObjectPath(opath))

	return h, nil
}

func (h *Hook) Start() error {
	call := h.object.Call(iface+".StartUnit", 0, h.unit, "fail")
	return call.Err
}

func (h *Hook) Subscribe() (chan *dbus.Signal, error) {
	h.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, "type='signal',interface='"+iface+"'")

	signals := make(chan *dbus.Signal, 10)
	h.conn.Signal(signals)

	err := h.object.Call(iface+".Subscribe", 0).Store()
	if err != nil {
		return nil, err
	}

	return signals, nil
}

func (h *Hook) getUnitState() (string, error) {
	var path dbus.ObjectPath
	var state dbus.Variant

	err := h.object.Call(iface+".GetUnit", 0, h.unit).Store(&path)
	if err != nil {
		return "", err
	}

	unit := h.conn.Object(dest, path)
	err = unit.Call("org.freedesktop.DBus.Properties.Get", 0, dest+".Unit", "ActiveState").Store(&state)
	if err != nil {
		return "", err
	}

	return state.Value().(string), nil
}

func (h *Hook) WatchForCompletion(signals chan *dbus.Signal, done chan error) {
	for signal := range signals {
		if signal.Name != iface+".JobRemoved" {
			continue
		}

		unit := signal.Body[2].(string)
		result := signal.Body[3].(string)

		if unit == h.unit && result == "done" {
			// our unit is done executing, make sure it succeeded
			state, err := h.getUnitState()
			if err != nil {
				done <- fmt.Errorf("error getting state of unit %q: %v", h.unit, err)
				return
			}

			if state == "failed" {
				done <- fmt.Errorf("failed to complete unit %q", h.unit)
				return
			}

			done <- nil
			return
		}
	}
}

func (h *Hook) CompleteUnit() error {
	signals, err := h.Subscribe()
	if err != nil {
		return err
	}

	err = h.Start()
	if err != nil {
		return err
	}

	done := make(chan error)
	go h.WatchForCompletion(signals, done)

	return <-done
}
