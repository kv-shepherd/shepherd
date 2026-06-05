# Shepherd Tempo

This directory contains the Compose Tempo configuration used by the built-in
monitoring overlay.

Tempo stores traces on the local Docker volume mounted at `/var/tempo` and
serves its HTTP query API on `http://tempo:3200`. Retention is intentionally
short for the starter stack; production retention and backup policies should be
set by the deployment owner.
