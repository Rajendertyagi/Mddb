<?php
/**
 * Plugin Name:       MDDB Sync
 * Plugin URI:        https://github.com/tradik/mddb
 * Description:       Synchronise WordPress posts and pages to an MDDB instance (https://mddb.tradik.com). On save → POST /v1/add; on delete/trash → POST /v1/delete. Detects post language via Polylang/WPML/WP locale.
 * Version:           0.1.1
 * Requires at least: 6.2
 * Requires PHP:      8.1
 * Author:            tradik
 * Author URI:        https://github.com/tradik
 * License:           BSD-3-Clause
 * License URI:       https://opensource.org/licenses/BSD-3-Clause
 * Text Domain:       mddb-sync
 * Domain Path:       /languages
 * Update URI:        https://github.com/tradik/mddb
 *
 * @package Tradik\MddbSync
 */

declare(strict_types=1);

defined( 'ABSPATH' ) || exit;

define( 'MDDB_SYNC_VERSION', '0.1.1' );
define( 'MDDB_SYNC_PLUGIN_FILE', __FILE__ );
define( 'MDDB_SYNC_PLUGIN_DIR', plugin_dir_path( __FILE__ ) );
define( 'MDDB_SYNC_PLUGIN_BASENAME', plugin_basename( __FILE__ ) );
define( 'MDDB_SYNC_GITHUB_REPO', 'tradik/mddb' );
define( 'MDDB_SYNC_GITHUB_ASSET_PREFIX', 'mddb-sync-' );

// phpcs:ignore WordPress.NamingConventions.PrefixAllGlobals.NonPrefixedVariableFound
$mddb_sync_autoload = __DIR__ . '/vendor/autoload.php';
if ( file_exists( $mddb_sync_autoload ) ) {
	require_once $mddb_sync_autoload;
} else {
	spl_autoload_register(
		static function ( string $class ): void {
			$prefix = 'Tradik\\MddbSync\\';
			if ( strpos( $class, $prefix ) !== 0 ) {
				return;
			}
			$relative = substr( $class, strlen( $prefix ) );
			$path     = MDDB_SYNC_PLUGIN_DIR . 'includes/class-' . strtolower(
				str_replace( [ '\\', '_' ], [ '-', '-' ], $relative )
			) . '.php';
			if ( file_exists( $path ) ) {
				require_once $path;
			}
		}
	);
}

add_action(
	'plugins_loaded',
	static function (): void {
		( new \Tradik\MddbSync\Plugin( MDDB_SYNC_VERSION ) )->boot();
	}
);

register_activation_hook(
	__FILE__,
	static function (): void {
		\Tradik\MddbSync\Plugin::onActivate();
	}
);

register_deactivation_hook(
	__FILE__,
	static function (): void {
		\Tradik\MddbSync\Plugin::onDeactivate();
	}
);
