#!/bin/bash -eu

# makes a postgres/sqlite shell to a demo homeserver
HS=$1
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

if [ $HS = "hs1" ]; then # postgres
    docker exec -it chaos-postgres1-1 /usr/local/bin/psql -U postgres synapse
elif [ $HS = "hs2" ]; then # sqlite
    sqlite3 "$SCRIPT_DIR/data/hs2/homeserver.db"
else
    echo "unknown homeserver $HS"
    exit 1
fi