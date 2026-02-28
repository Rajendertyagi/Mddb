<?php
require 'mddb.php';

$mddb = mddb::connect('localhost:11023','read');  // tryb klienta
$mddb = $mddb->collection('blog');
$mddb = $mddb->env('year','2004');

$homepage_content = $mddb->get('homepage','en_GB');  // %%year%% będzie podmienione

$posts = $mddb->search('category','blog','addedAt', true);

foreach ($posts as $post) {
  echo "<h2>" . htmlspecialchars($post->key) . "</h2>";
}

// --- Vector Search ---

// Semantic search - znajdź artykuły po znaczeniu
$results = $mddb->vectorSearch('jak logować użytkowników', 5, 0.3, true);

echo "\nVector Search Results:\n";
foreach ($results->results as $r) {
  echo "#" . $r->rank . " " . round($r->score * 100) . "% " . $r->document->key . "\n";
}

// Sprawdź status embeddingów
$stats = $mddb->vectorStats();
echo "\nEmbedding provider: " . ($stats->enabled ? $stats->model : 'disabled') . "\n";

// Reindeksuj kolekcję (tryb write)
// $mddb_write = mddb::connect('localhost:11023','write')->collection('blog');
// $mddb_write->vectorReindex(false);
