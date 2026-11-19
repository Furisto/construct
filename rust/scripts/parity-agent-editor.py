#!/usr/bin/env python3
"""Noninteractive $EDITOR fixture for the agent parity scenarios."""
from pathlib import Path
import os
import sys

import yaml

path = Path(sys.argv[1])
spec = yaml.safe_load(path.read_text())
spec.update(name='edited-agent', description='Edited description',
            instructions='Edited prompt', model=os.environ['PARITY_EDIT_MODEL'])
path.write_text(yaml.safe_dump(spec))
