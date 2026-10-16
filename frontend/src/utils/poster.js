const PALETTE = [
  ['#6a3de8', '#2b1a5e'],
  ['#e8503d', '#5e1a2b'],
  ['#3dc9e8', '#1a4e5e'],
  ['#e8a13d', '#5e3f1a'],
  ['#3de87a', '#1a5e35'],
  ['#e83d9e', '#5e1a45'],
]

export function posterGradient(seed) {
  let hash = 0
  for (let i = 0; i < seed.length; i++) {
    hash = (hash * 31 + seed.charCodeAt(i)) >>> 0
  }
  const [from, to] = PALETTE[hash % PALETTE.length]
  return `linear-gradient(160deg, ${from}, ${to})`
}
