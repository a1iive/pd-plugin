#! /bin/bash
n=$1
tikvdir="."

if [ $n -gt 100 ]
then
	echo "tikv-num = $n is too big"
else
	echo "start $n tikv!"
	for((i=1;i<=$n;i++))
	do
	{
		$tikvdir/bin/tikv-server --pd-endpoints="127.0.0.1:2379" \
                	--addr="127.0.0.1:`expr $i + 20159`" \
	                --data-dir=tikv$i \
        	        --log-file=tikv$i.log \
			--labels zone=z$i,rack=r$i,host=h$i

	}&
	done
fi
