function ipv4Value(address) {
  if (!/^\d{1,3}(\.\d{1,3}){3}$/.test(address)) return null;
  const octets = address.split('.').map(Number);
  if (octets.some(octet => octet > 255)) return null;
  return octets.reduce((value, octet) => value * 256 + octet, 0);
}

export function matchesResource(resource, keyword) {
  const target = String(resource.target || resource.Target || '').toLowerCase();
  const kind = String(resource.kind || resource.Kind || '').toLowerCase();
  const ip = ipv4Value(keyword);
  if (ip === null || kind !== 'ip') return target.includes(keyword);

  const bounds = target.split('~').map(value => value.trim());
  if (bounds.length === 1) return ipv4Value(bounds[0]) === ip;
  if (bounds.length !== 2) return false;
  const start = ipv4Value(bounds[0]);
  const end = ipv4Value(bounds[1]);
  return start !== null && end !== null && start <= ip && ip <= end;
}
