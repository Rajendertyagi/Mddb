<?php
/**
 * Bootstraps every collaborator (settings, sync, updater).
 *
 * @package Tradik\MddbSync
 */

declare(strict_types=1);

namespace Tradik\MddbSync;

defined( 'ABSPATH' ) || exit;

/**
 * Plugin orchestrator — wires every collaborator and exposes the activation hooks.
 */
final class Plugin {

	public const OPTION_NAME = 'mddb_sync_options';

	private string $version;

	public function __construct( string $version ) {
		$this->version = $version;
	}

	/**
	 * Wire every collaborator. Called from the `plugins_loaded` hook so that
	 * Polylang/WPML hooks are already registered.
	 */
	public function boot(): void {
		load_plugin_textdomain( 'mddb-sync', false, dirname( MDDB_SYNC_PLUGIN_BASENAME ) . '/languages' );

		$options   = new Settings();
		$language  = new Language();
		$client    = new Client( $options );
		$mapper    = new Mapper( $language );
		$sync      = new Sync( $options, $client, $mapper );
		$bulk      = new Bulk( $options, $sync );
		$admin     = new Admin( $options, $client, $bulk );
		$updater   = new Updater( MDDB_SYNC_PLUGIN_BASENAME, $this->version, MDDB_SYNC_GITHUB_REPO );
		$publisher = new Publisher( $options, new Markdown(), new Translations() );
		$rest      = new Rest( $options, $publisher );

		$sync->register();
		$admin->register();
		$updater->register();
		$rest->register();
	}

	/**
	 * One-time defaults so the settings page renders without surprises.
	 */
	public static function onActivate(): void {
		if ( ! get_option( self::OPTION_NAME ) ) {
			add_option( self::OPTION_NAME, Settings::defaults() );
		}
	}

	/**
	 * Tear-down: clear the GitHub-release cache only. User-visible
	 * options stay so deactivate/activate cycles keep configuration.
	 */
	public static function onDeactivate(): void {
		delete_site_transient( Updater::TRANSIENT_KEY );
	}
}
