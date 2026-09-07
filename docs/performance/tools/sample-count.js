// mongosh script: samples the count of documents created by
// direct-publish/ingest-load (title "Ingest Load Test") every ~15ms,
// entirely inside one mongosh process — spawning a new `docker compose
// exec` per sample would itself dominate the interval at this
// resolution. Used to measure real throughput of movies-service's
// RabbitMQ consumer (see docs/adr/0005-*.md and docs/performance/README.md,
// "Fase 2b").
//
// Usage: run BEFORE starting the publisher, so it's already sampling by
// t=0. It exits once the count holds steady for 15 consecutive samples
// (assumed drained) or after 60s, whichever comes first — tune both via
// the constants below for a bigger/smaller expected batch.
//
//   docker compose exec -T mongo mongosh --quiet -u "$MONGO_ROOT_USER" \
//     -p "$MONGO_ROOT_PASSWORD" --authenticationDatabase admin \
//     --eval "$(cat docs/performance/tools/sample-count.js)" \
//     > out.txt &
//   sleep 0.5
//   (cd docs/performance/tools/direct-publish && go run . -n 100000)
db = db.getSiblingDB("moviesDB");
let start = new Date();
let last = -1;
let stable = 0;
while (true) {
  let c = db.movies.countDocuments({ title: "Ingest Load Test" });
  let elapsed = (new Date() - start) / 1000;
  print("t=" + elapsed.toFixed(3) + "s count=" + c);
  if (c === last) {
    stable++;
  } else {
    stable = 0;
  }
  last = c;
  if (stable >= 15 && c > 0) break;
  if (elapsed > 60) break;
  sleep(15);
}
