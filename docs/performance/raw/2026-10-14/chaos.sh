#!/bin/bash
# uso: chaos.sh <nome> <targets> <rate> <dur> <t_falha> <cmd_falha> <t_volta> <cmd_volta>
S="${S:-$(pwd)}"  # diretório de trabalho com targets/ e load/
mkdir -p "$S/load"
n=$1; t=$2; r=$3; d=$4; tf=$5; cf=$6; tv=$7; cv=$8
~/go/bin/vegeta attack -targets=$S/targets/$t.txt -rate=$r -duration=$d -timeout=10s > $S/load/$n.bin &
VP=$!
sleep $tf; echo "[t=${tf}s] FALHA: $cf"; bash -c "$cf" >/dev/null 2>&1
sleep $((tv-tf)); echo "[t=${tv}s] VOLTA: $cv"; bash -c "$cv" >/dev/null 2>&1
wait $VP
~/go/bin/vegeta report $S/load/$n.bin > $S/load/$n.txt
~/go/bin/vegeta report -type=json $S/load/$n.bin | python3 $S/summ.py
$S/timeline.sh $S/load/$n.bin > $S/load/$n.timeline
