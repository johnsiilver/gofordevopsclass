#!/bin/sh

test -d "$TMPDIR"
tmpDirSet=$?

if [ "$tmpDirSet" != "0" ]; then
    echo "TMPDIR is not set: " 
    echo $tmpDirSet
    exit 1
fi

# Start our load balancer on 9090(http) and 9091(grpc)
echo "Starting load balancer"
cd lb/
go build -o classLB
./classLB &


# Start 9 web servers on ports 9092-9100
cd sample/web
cwd=$(pwd)

echo "Building web server"
go build -o web

echo "Starting web servers"
mkdir -p $TMPDIR/workflowExample/instance0
cp web $TMPDIR/workflowExample/instance0/classWeb0
$TMPDIR/workflowExample/instance0/classWeb0 --port=9092 &
mkdir -p $TMPDIR/workflowExample/instance1
cp web $TMPDIR/workflowExample/instance1/classWeb1
$TMPDIR/workflowExample/instance1/classWeb1 --port=9093 &
mkdir -p $TMPDIR/workflowExample/instance2
cp web $TMPDIR/workflowExample/instance2/classWeb2
$TMPDIR/workflowExample/instance2/classWeb2 --port=9094 &
mkdir -p $TMPDIR/workflowExample/instance3
cp web $TMPDIR/workflowExample/instance3/classWeb3
$TMPDIR/workflowExample/instance3/classWeb3 --port=9095 &
mkdir -p $TMPDIR/workflowExample/instance4
cp web $TMPDIR/workflowExample/instance4/classWeb4
$TMPDIR/workflowExample/instance4/classWeb4 --port=9096 &
mkdir -p $TMPDIR/workflowExample/instance5
cp web $TMPDIR/workflowExample/instance5/classWeb5
$TMPDIR/workflowExample/instance5/classWeb5 --port=9097 &
mkdir -p $TMPDIR/workflowExample/instance6
cp web $TMPDIR/workflowExample/instance6/classWeb6
$TMPDIR/workflowExample/instance6/classWeb6 --port=9098 &
mkdir -p $TMPDIR/workflowExample/instance7
cp web $TMPDIR/workflowExample/instance7/classWeb7
$TMPDIR/workflowExample/instance7/classWeb7 --port=9099 &

# Get us back to the original directory.
cd $cwd

# Build our replacement binary.
echo "Building replacement binary"
cd ../replace
go build -o web

# User our CLI client for the load balancer to add a pool and backends
echo "Adding pool and backends to load balancer"
cd ../../client/cli
go run cli.go --lb=127.0.0.1:9091 --pattern=/ addPool
go run cli.go --lb=127.0.0.1:9091 --pattern=/ --ip=127.0.0.1 --port=9092 --url_path=/ addBackend
go run cli.go --lb=127.0.0.1:9091 --pattern=/ --ip=127.0.0.1 --port=9093 --url_path=/ addBackend
go run cli.go --lb=127.0.0.1:9091 --pattern=/ --ip=127.0.0.1 --port=9094 --url_path=/ addBackend
go run cli.go --lb=127.0.0.1:9091 --pattern=/ --ip=127.0.0.1 --port=9095 --url_path=/ addBackend
go run cli.go --lb=127.0.0.1:9091 --pattern=/ --ip=127.0.0.1 --port=9096 --url_path=/ addBackend
go run cli.go --lb=127.0.0.1:9091 --pattern=/ --ip=127.0.0.1 --port=9097 --url_path=/ addBackend
go run cli.go --lb=127.0.0.1:9091 --pattern=/ --ip=127.0.0.1 --port=9098 --url_path=/ addBackend
go run cli.go --lb=127.0.0.1:9091 --pattern=/ --ip=127.0.0.1 --port=9099 --url_path=/ addBackend

sleep 3

# Use our CLI client to check the health of the pool
go run cli.go --lb=127.0.0.1:9091 --pattern=/ poolHealth

# To kill all the processes
# pkill -f 'classWeb'; pkill -f 'classLB'

# Remove files created in tmpdir
# rm -rf $TMPDIR/workflowExample/