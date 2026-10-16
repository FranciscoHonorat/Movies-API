#!/bin/bash
# timeline por segundo: total / ok / erros, a partir de um .bin do vegeta
~/go/bin/vegeta encode --to csv "$1" | python3 -c '
import sys,csv,collections
rows=[(int(r[0]),r[1]) for r in csv.reader(sys.stdin)]
t0=min(r[0] for r in rows); b=collections.defaultdict(lambda:[0,0])
for ts,c in rows:
    s=(ts-t0)//10**9; b[s][0]+=1; b[s][1]+= c=="200" or c=="202" or c=="204"
for s in sorted(b): print(f"{s}s:{b[s][1]}ok/{b[s][0]-b[s][1]}err")
'
