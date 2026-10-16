import json,sys
r=json.load(open(sys.argv[1])) if len(sys.argv)>1 else json.load(sys.stdin)
L=r["latencies"]
print(f"req={r['requests']} ok={r['success']*100:.2f}% codes={r['status_codes']} p50={L['50th']/1e6:.2f}ms p99={L['99th']/1e6:.1f}ms max={L['max']/1e6:.0f}ms")
