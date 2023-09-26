#!/bin/sh

cd ../../../../cli_apps/qotd/server/
go build -o server .
mv server ../../../automation_the_hard_way/agent/client/cli/
cd ../../../automation_the_hard_way/agent/client/cli/
zip qotd.zip server
rm server
