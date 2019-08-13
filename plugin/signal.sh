#! /bin/bash
sudo kill -SIGUSR1 $(ps -ef|grep /bin/pd-server |gawk '$0 !~/grep/ {print $2}' |tr -s '\n' ' ')

