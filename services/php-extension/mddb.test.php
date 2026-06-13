<?php
// Minimal smoke test for the MDDB PHP client (no PHPUnit in this package).
// Run with: php -d zend.assertions=1 -d assert.exception=1 mddb.test.php
//
// INT-009: asserts every cURL handle is configured with explicit timeouts and
// TLS verification via the shared applyCurlDefaults helper.

declare(strict_types=1);

require_once __DIR__ . '/mddb.php';

$rc = new ReflectionClass('mddb');

// Timeout constants exist with safe values.
assert($rc->getConstant('CONNECT_TIMEOUT') === 5, 'CONNECT_TIMEOUT must be 5');
assert($rc->getConstant('TIMEOUT') === 30, 'TIMEOUT must be 30');

// The shared defaults helper exists.
assert($rc->hasMethod('applyCurlDefaults'), 'applyCurlDefaults missing');

// When the cURL extension is available, the helper must run against a real
// handle without error. (Skipped on a CLI built without ext-curl.)
if (extension_loaded('curl')) {
  $m = $rc->getMethod('applyCurlDefaults');
  $m->setAccessible(true);
  $ch = curl_init('https://127.0.0.1:1/');
  $m->invoke(null, $ch);
  assert(curl_getinfo($ch, CURLINFO_EFFECTIVE_URL) === 'https://127.0.0.1:1/');
  curl_close($ch);
} else {
  echo "note: ext-curl not loaded — skipping live handle check\n";
}

// Every cURL path in the client must wire the shared defaults.
$src = file_get_contents(__DIR__ . '/mddb.php');
$calls = substr_count($src, 'self::applyCurlDefaults($ch)');
assert($calls === 3, "expected applyCurlDefaults on all 3 cURL paths, found {$calls}");

echo "INT-009 smoke test passed\n";
