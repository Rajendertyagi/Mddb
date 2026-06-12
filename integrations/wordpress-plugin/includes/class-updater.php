<?php
/**
 * Checks GitHub Releases for plugin updates.
 *
 * @package Tradik\MddbSync
 */

declare(strict_types=1);

namespace Tradik\MddbSync;

defined( 'ABSPATH' ) || exit;

/**
 * Self-hosted update channel.
 *
 * WordPress doesn't talk to wp.org for plugins shipped from a private repo,
 * so we hook into the same filters wp.org would and source the metadata
 * from `repos/<owner>/<repo>/releases/latest`. The release asset must be
 * named `mddb-sync-<version>.zip` and ship the same plugin layout that
 * lives in this folder.
 *
 * Responses are cached in a site transient for 12h.
 */
final class Updater {

	public const TRANSIENT_KEY = 'mddb_sync_latest_release';

	private const TRANSIENT_TTL = 12 * HOUR_IN_SECONDS;

	private string $basename;

	private string $currentVersion;

	private string $repo;

	public function __construct( string $basename, string $currentVersion, string $repo ) {
		$this->basename       = $basename;
		$this->currentVersion = $currentVersion;
		$this->repo           = $repo;
	}

	public function register(): void {
		add_filter( 'pre_set_site_transient_update_plugins', [ $this, 'injectUpdate' ] );
		add_filter( 'plugins_api', [ $this, 'providePluginInformation' ], 10, 3 );
	}

	/**
	 * @param mixed $transient
	 * @return mixed
	 */
	public function injectUpdate( $transient ) {
		if ( ! is_object( $transient ) ) {
			return $transient;
		}
		$release = $this->latestRelease();
		if ( $release === null ) {
			return $transient;
		}
		if ( version_compare( $release['version'], $this->currentVersion, '<=' ) ) {
			return $transient;
		}

		$payload = (object) [
			'id'            => $this->basename,
			'slug'          => dirname( $this->basename ),
			'plugin'        => $this->basename,
			'new_version'   => $release['version'],
			'url'           => $release['htmlUrl'],
			'package'       => $release['zipUrl'],
			'tested'        => $release['tested'],
			'requires_php'  => $release['requiresPhp'],
			'compatibility' => new \stdClass(),
		];

		if ( ! isset( $transient->response ) || ! is_array( $transient->response ) ) {
			$transient->response = [];
		}
		$transient->response[ $this->basename ] = $payload;
		return $transient;
	}

	/**
	 * @param mixed                 $result
	 * @param string                $action
	 * @param object|array<mixed>   $args
	 * @return mixed
	 */
	public function providePluginInformation( $result, string $action, $args ) {
		if ( $action !== 'plugin_information' ) {
			return $result;
		}
		$slug = is_object( $args ) ? (string) ( $args->slug ?? '' ) : (string) ( $args['slug'] ?? '' );
		if ( $slug !== dirname( $this->basename ) ) {
			return $result;
		}
		$release = $this->latestRelease();
		if ( $release === null ) {
			return $result;
		}
		return (object) [
			'name'          => 'MDDB Sync',
			'slug'          => $slug,
			'version'       => $release['version'],
			'author'        => '<a href="https://github.com/tradik">tradik</a>',
			'homepage'      => $release['htmlUrl'],
			'download_link' => $release['zipUrl'],
			'requires_php'  => $release['requiresPhp'],
			'tested'        => $release['tested'],
			// INT-003: GitHub release notes are rendered as HTML in the wp-admin
			// "View details" modal. Run them through wp_kses_post() so only
			// post-safe markup survives (no <script>, event handlers, etc.).
			'sections'      => [
				'description' => wp_kses_post( $release['body'] ),
				'changelog'   => wp_kses_post( $release['body'] ),
			],
		];
	}

	/**
	 * @return array{version:string,tagName:string,htmlUrl:string,zipUrl:string,body:string,requiresPhp:string,tested:string}|null
	 */
	public function latestRelease(): ?array {
		$cached = get_site_transient( self::TRANSIENT_KEY );
		if ( is_array( $cached ) && isset( $cached['version'] ) ) {
			return $cached;
		}

		$response = wp_remote_get(
			sprintf( 'https://api.github.com/repos/%s/releases/latest', $this->repo ),
			[
				'timeout' => 15,
				'headers' => [
					'Accept'     => 'application/vnd.github+json',
					'User-Agent' => 'mddb-sync/' . $this->currentVersion,
				],
			]
		);
		if ( is_wp_error( $response ) ) {
			set_site_transient( self::TRANSIENT_KEY, [ 'error' => $response->get_error_message() ], MINUTE_IN_SECONDS * 30 );
			return null;
		}
		$code = (int) wp_remote_retrieve_response_code( $response );
		if ( $code !== 200 ) {
			set_site_transient( self::TRANSIENT_KEY, [ 'error' => "HTTP $code" ], MINUTE_IN_SECONDS * 30 );
			return null;
		}
		$body = json_decode( (string) wp_remote_retrieve_body( $response ), true );
		if ( ! is_array( $body ) ) {
			return null;
		}

		$tag    = (string) ( $body['tag_name'] ?? '' );
		$assets = is_array( $body['assets'] ?? null ) ? $body['assets'] : [];

		$zipUrl = '';
		foreach ( $assets as $asset ) {
			$name = (string) ( $asset['name'] ?? '' );
			if ( strpos( $name, MDDB_SYNC_GITHUB_ASSET_PREFIX ) === 0 && substr( $name, -4 ) === '.zip' ) {
				$zipUrl = (string) ( $asset['browser_download_url'] ?? '' );
				break;
			}
		}

		$release = [
			'version'     => ltrim( $tag, 'vV' ),
			'tagName'     => $tag,
			'htmlUrl'     => (string) ( $body['html_url'] ?? '' ),
			'zipUrl'      => $zipUrl,
			'body'        => (string) ( $body['body'] ?? '' ),
			'requiresPhp' => '8.1',
			'tested'      => '6.7',
		];
		set_site_transient( self::TRANSIENT_KEY, $release, self::TRANSIENT_TTL );
		return $release;
	}
}
