#!/bin/bash
# uso: run.sh <nome> <targets> <rate> <duration> [extra vegeta args]
S="${S:-$(pwd)}"  # diretório de trabalho com targets/ e load/
mkdir -p "$S/load"
name=$1; t=$2; rate=$3; dur=$4; shift 4
( while true; do docker stats --no-stream --format '{{.Name}} {{.CPUPerc}} {{.MemUsage}}' movies-mongo movies-service api-gateway movies-rabbitmq; sleep 2; done ) > $S/load/$name.stats 2>/dev/null &
SP=$!
~/go/bin/vegeta attack -targets=$S/targets/$t.txt -rate=$rate -duration=$dur -timeout=10s "$@" > $S/load/$name.bin
kill $SP 2>/dev/null; wait $SP 2>/dev/null
~/go/bin/vegeta report $S/load/$name.bin > $S/load/$name.txt
~/go/bin/vegeta report -type=json $S/load/$name.bin > $S/load/$name.json
python3 - $S/load/$name.json $S/load/$name.stats "$name" "$rate" <<'PY'
import json,sys,collections
r=json.load(open(sys.argv[1])); L=r["latencies"]
f=lambda ns: f"{ns/1e6:.2f}ms"
peak=collections.defaultdict(float)
for line in open(sys.argv[2]):
    p=line.split()
    if len(p)>=2 and p[1].endswith('%'):
        peak[p[0]]=max(peak[p[0]],float(p[1][:-1]))
print(f"{sys.argv[3]:<22} rate={sys.argv[4]:>5} req={r['requests']:>6} thr={r['throughput']:.0f}/s ok={r['success']*100:.2f}% mean={f(L['mean'])} p50={f(L['50th'])} p95={f(L['95th'])} p99={f(L['99th'])} max={f(L['max'])} codes={r['status_codes']} cpu_peak={dict((k,round(v)) for k,v in peak.items())}")
PY
