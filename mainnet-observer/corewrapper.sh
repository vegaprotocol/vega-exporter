#!/bin/sh

set -- /vega/vega/state/node/snapshots/snapshot.db/*.ldb
if [ -f "$1" ]
then
    START_WITH_SNAPSHOT_ARG="--snapshot.load-from-block-height=-1"
else
    START_WITH_SNAPSHOT_ARG=""
fi

vega node --home=/vega/vega --nodewallet-passphrase-file=/vega/passphrase $START_WITH_SNAPSHOT_ARG

