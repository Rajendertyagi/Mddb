<?php

/**
 * MDDB PHP client.
 *
 * TCP:   mddb::connect('localhost:11023', 'read')
 * UDS:   mddb::connect('unix:/tmp/mddb.sock', 'read')   // MDDB 2.9.13+
 */
class mddb
{
  private string $base;
  private string $mode;
  private string $collection = '';
  private array $env = [];
  private ?string $unixSocket = null;

  public static function connect(string $addr, string $mode = 'read'): self
  {
    $i = new self;
    $i->mode = $mode;
    if (strncmp($addr, 'unix:', 5) === 0) {
      $path = substr($addr, 5);
      if (strncmp($path, '//', 2) === 0) {
        $path = substr($path, 2);
      }
      $i->unixSocket = $path;
      $i->base = 'http://localhost/v1';
    } else {
      $i->base = "http://$addr/v1";
    }
    return $i;
  }

  public function collection(string $name): self
  {
    $this->collection = $name;
    return $this;
  }

  public function env(string $k, string $v): self
  {
    $this->env[$k] = $v;
    return $this;
  }

  public function get(string $key, string $lang)
  {
    $payload = [
      'collection' => $this->collection,
      'key' => $key,
      'lang' => $lang,
      'env' => $this->env
    ];
    return $this->post('/get', $payload);
  }

  public function add(string $key, string $lang, array $meta, string $contentMd)
  {
    if ($this->mode === 'read')
      throw new Exception("read-only client");
    $payload = [
      'collection' => $this->collection,
      'key' => $key,
      'lang' => $lang,
      'meta' => $meta,
      'contentMd' => $contentMd
    ];
    return $this->post('/add', $payload);
  }

  public function search(string $metaKey, string $metaVal, string $sort = 'addedAt', bool $asc = true, int $limit = 100)
  {
    $payload = [
      'collection' => $this->collection,
      'filterMeta' => [$metaKey => [$metaVal]],
      'sort' => $sort,
      'asc' => $asc,
      'limit' => $limit,
      'offset' => 0
    ];
    return $this->post('/search', $payload);
  }

  public function vectorSearch(string $query, int $topK = 5, float $threshold = 0.0, bool $includeContent = false, ?array $filterMeta = null)
  {
    $payload = [
      'collection' => $this->collection,
      'query' => $query,
      'topK' => $topK,
      'threshold' => $threshold,
      'includeContent' => $includeContent,
    ];
    if ($filterMeta !== null) {
      $payload['filterMeta'] = $filterMeta;
    }
    return $this->post('/vector-search', $payload);
  }

  public function vectorReindex(bool $force = false)
  {
    $payload = [
      'collection' => $this->collection,
      'force' => $force,
    ];
    return $this->post('/vector-reindex', $payload);
  }

  public function vectorStats()
  {
    return $this->httpGet('/vector-stats');
  }

  public function importUrl(string $url, string $lang, ?string $key = null, ?array $meta = null, int $ttl = 0)
  {
    if ($this->mode === 'read')
      throw new Exception("read-only client");
    $payload = [
      'collection' => $this->collection,
      'url' => $url,
      'lang' => $lang,
    ];
    if ($key !== null)
      $payload['key'] = $key;
    if ($meta !== null)
      $payload['meta'] = $meta;
    if ($ttl > 0)
      $payload['ttl'] = $ttl;
    return $this->post('/import-url', $payload);
  }

  public function setTtl(string $key, string $lang, int $ttl)
  {
    if ($this->mode === 'read')
      throw new Exception("read-only client");
    return $this->post('/set-ttl', [
      'collection' => $this->collection,
      'key' => $key,
      'lang' => $lang,
      'ttl' => $ttl,
    ]);
  }

  public function ftsSearch(string $query, int $limit = 50)
  {
    return $this->post('/fts', [
      'collection' => $this->collection,
      'query' => $query,
      'limit' => $limit,
    ]);
  }

  public function registerWebhook(string $url, array $events, string $collection = '')
  {
    if ($this->mode === 'read')
      throw new Exception("read-only client");
    $payload = ['url' => $url, 'events' => $events];
    if ($collection !== '')
      $payload['collection'] = $collection;
    return $this->post('/webhooks', $payload);
  }

  public function listWebhooks()
  {
    return $this->httpGet('/webhooks');
  }

  public function deleteWebhook(string $id)
  {
    if ($this->mode === 'read')
      throw new Exception("read-only client");
    return $this->post('/webhooks/delete', ['id' => $id]);
  }

  public function setSchema(array $schema)
  {
    if ($this->mode === 'read')
      throw new Exception("read-only client");
    return $this->post('/schema/set', [
      'collection' => $this->collection,
      'schema' => $schema,
    ]);
  }

  public function getSchema()
  {
    return $this->post('/schema/get', [
      'collection' => $this->collection,
    ]);
  }

  public function deleteSchema()
  {
    if ($this->mode === 'read')
      throw new Exception("read-only client");
    return $this->post('/schema/delete', [
      'collection' => $this->collection,
    ]);
  }

  public static function listSchemas(string $addr)
  {
    $base = "http://$addr/v1";
    $ch = curl_init($base . '/schema/list');
    curl_setopt($ch, CURLOPT_POST, true);
    curl_setopt($ch, CURLOPT_HTTPHEADER, ['Content-Type: application/json']);
    curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode([]));
    curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
    $res = curl_exec($ch);
    if ($res === false)
      throw new Exception(curl_error($ch));
    $code = curl_getinfo($ch, CURLINFO_HTTP_CODE);
    curl_close($ch);
    if ($code >= 400)
      throw new Exception($res);
    return json_decode($res);
  }

  public function validate(array $meta)
  {
    return $this->post('/validate', [
      'collection' => $this->collection,
      'meta' => $meta,
    ]);
  }

  private function httpGet(string $path)
  {
    $ch = curl_init($this->base . $path);
    curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
    if ($this->unixSocket !== null) {
      curl_setopt($ch, CURLOPT_UNIX_SOCKET_PATH, $this->unixSocket);
    }
    $res = curl_exec($ch);
    if ($res === false)
      throw new Exception(curl_error($ch));
    $code = curl_getinfo($ch, CURLINFO_HTTP_CODE);
    curl_close($ch);
    if ($code >= 400)
      throw new Exception($res);
    return json_decode($res);
  }

  private function post(string $path, array $payload)
  {
    $ch = curl_init($this->base . $path);
    curl_setopt($ch, CURLOPT_POST, true);
    curl_setopt($ch, CURLOPT_HTTPHEADER, ['Content-Type: application/json']);
    curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode($payload));
    curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
    if ($this->unixSocket !== null) {
      curl_setopt($ch, CURLOPT_UNIX_SOCKET_PATH, $this->unixSocket);
    }
    $res = curl_exec($ch);
    if ($res === false)
      throw new Exception(curl_error($ch));
    $code = curl_getinfo($ch, CURLINFO_HTTP_CODE);
    curl_close($ch);
    if ($code >= 400)
      throw new Exception($res);
    return json_decode($res);
  }
}
