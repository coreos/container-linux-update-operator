# Reboot windows
The CLUO `update-operator` can be configured to only reboot nodes during certain timeframes.
Pre-reboot checks are prevented from running as well.

## Configuring update-operator

The reboot window is configured through the the flags `--reboot-window-start` 
and `--reboot-window-length`, or through the environment variables 
`UPDATE_OPERATOR_REBOOT_WINDOW_START` and `UPDATE_OPERATOR_REBOOT_WINDOW_LENGTH`.

Here is an example configuration:

```
/bin/update-operator \
 --reboot-window-start=14:00 \
 --reboot-window-length=1h
```

This would configure `update-operator` to only reboot between 2pm and 3pm. Optionally,
a day of week may be specified for the start of the window:

```
/bin/update-operator \
 --reboot-window-start="Thu 23:00" \
 --reboot-window-length=1h30m
```

This would configure `update-operator` to only reboot the system on Thursday after 11pm,
or on Friday before 12:30am.

Currently, the only supported values for the day of week are short day names,
e.g. `Sun`, `Mon`, `Tue`, `Wed`, `Thu`, `Fri`, and `Sat`, but the day of week can
be upper or lower case. The time of day must be specified in 24-hour time format.
The window length is expressed as input to go's [time.ParseDuration][time.ParseDuration]
function.

[time.ParseDuration]: http://godoc.org/time#ParseDuration
