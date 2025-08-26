# mailezine HTTP checkv2 endpoint (the stock normal worker on 11333 is
# disabled by local.d; mailezine talks HTTP, not milter, so re-enable a
# normal scanner on 11333).
enabled = true;
bind_socket = "*:11333";
mime = true;
