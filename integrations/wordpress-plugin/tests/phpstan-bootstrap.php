<?php
/**
 * Bootstrap for PHPStan: declare plugin constants so static analysis can
 * resolve `MDDB_SYNC_VERSION` and friends.
 *
 * @package Tradik\MddbSync
 */

declare(strict_types=1);

defined( 'MDDB_SYNC_VERSION' ) || define( 'MDDB_SYNC_VERSION', '0.1.0' );
defined( 'MDDB_SYNC_PLUGIN_BASENAME' ) || define( 'MDDB_SYNC_PLUGIN_BASENAME', 'mddb-sync/mddb-sync.php' );
defined( 'MDDB_SYNC_PLUGIN_DIR' ) || define( 'MDDB_SYNC_PLUGIN_DIR', __DIR__ . '/../' );
defined( 'MDDB_SYNC_GITHUB_REPO' ) || define( 'MDDB_SYNC_GITHUB_REPO', 'tradik/mddb' );
defined( 'MDDB_SYNC_GITHUB_ASSET_PREFIX' ) || define( 'MDDB_SYNC_GITHUB_ASSET_PREFIX', 'mddb-sync-' );
