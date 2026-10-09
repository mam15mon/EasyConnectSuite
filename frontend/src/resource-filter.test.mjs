import assert from 'node:assert/strict';
import test from 'node:test';
import { matchesResource } from './resource-filter.mjs';

test('完整 IP 搜索命中包含该地址的范围，并包含边界', () => {
  const resource = { kind: 'ip', target: '172.31.0.1 ~ 172.31.127.254' };
  for (const ip of ['172.31.105.240', '172.31.0.1', '172.31.127.254']) {
    assert.equal(matchesResource(resource, ip), true, ip);
  }
  for (const ip of ['172.31.0.0', '172.31.127.255', '172.31.128.1', '172.30.105.240']) {
    assert.equal(matchesResource(resource, ip), false, ip);
  }
});

test('单个 IP 精确匹配，完整地址不会误匹配范围端点的文字前缀', () => {
  assert.equal(matchesResource({ kind: 'ip', target: '172.31.105.240' }, '172.31.105.240'), true);
  assert.equal(matchesResource({ kind: 'ip', target: '172.31.105.240' }, '172.31.105.24'), false);
  assert.equal(matchesResource({ kind: 'ip', target: '172.31.127.254 ~ 172.31.128.1' }, '172.31.127.25'), false);
});

test('保留部分 IP、域名及固定映射的文字搜索', () => {
  assert.equal(matchesResource({ kind: 'ip', target: '172.31.0.1 ~ 172.31.127.254' }, '172.31.1'), true);
  assert.equal(matchesResource({ kind: 'domain', target: 'Console.Internal.Example → 172.31.105.240' }, 'console.internal'), true);
  assert.equal(matchesResource({ kind: 'domain', target: 'console.internal.example → 172.31.105.240' }, '172.31.105.240'), true);
});

test('IPv4 数值比较支持高位地址和旧字段名称', () => {
  assert.equal(matchesResource({ Kind: 'ip', Target: '198.19.0.1 ~ 255.255.255.254' }, '255.255.255.253'), true);
  assert.equal(matchesResource({ kind: 'ip', target: 'invalid ~ 172.31.127.254' }, '172.31.105.240'), false);
});
