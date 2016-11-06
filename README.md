This project is a Serial-Over-IP driver. It can be configured via the config file you may find in the distribution.
I'd like to enhance it so that if you try to connect to a port in use you receive a connection refused message and not connected to a
non-accepting socket until TCP timeout.
anyway it work fine, at least on Windows. It makes use of the TARM serial library.
