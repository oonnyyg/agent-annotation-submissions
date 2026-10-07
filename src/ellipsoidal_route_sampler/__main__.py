"""Allow `python -m ellipsoidal_route_sampler`."""

import sys

from .cli import run

if __name__ == "__main__":
    sys.exit(run())
