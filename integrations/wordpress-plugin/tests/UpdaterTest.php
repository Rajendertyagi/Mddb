<?php
/**
 * Tests for the GitHub-Releases updater.
 *
 * @package Tradik\MddbSync\Tests
 */

declare(strict_types=1);

namespace Tradik\MddbSync\Tests;

use Brain\Monkey;
use Brain\Monkey\Functions;
use PHPUnit\Framework\TestCase;
use Tradik\MddbSync\Updater;
use WP_Error;

final class UpdaterTest extends TestCase {

	protected function setUp(): void {
		parent::setUp();
		Monkey\setUp();
		Functions\when( 'add_filter' )->justReturn( true );
	}

	protected function tearDown(): void {
		Monkey\tearDown();
		parent::tearDown();
	}

	private function releaseFixture( string $tag = 'v0.2.0' ): array {
		return [
			'tag_name' => $tag,
			'html_url' => 'https://github.com/tradik/mddb/releases/tag/' . $tag,
			'body'     => 'changelog body',
			'assets'   => [
				[
					'name'                 => 'mddb-sync-' . ltrim( $tag, 'vV' ) . '.zip',
					'browser_download_url' => 'https://github.com/tradik/mddb/releases/download/' . $tag . '/mddb-sync.zip',
				],
			],
		];
	}

	public function testInjectUpdateSkipsWhenNoNewVersion(): void {
		Functions\when( 'get_site_transient' )->justReturn(
			[
				'version' => '0.1.0',
				'tagName' => 'v0.1.0',
				'htmlUrl' => '',
				'zipUrl'  => 'https://example.com/asset.zip',
				'body'    => '',
				'requiresPhp' => '8.1',
				'tested'  => '6.7',
			]
		);
		$updater = new Updater( 'mddb-sync/mddb-sync.php', '0.1.0', 'tradik/mddb' );
		$transient = new \stdClass();
		$result    = $updater->injectUpdate( $transient );
		self::assertSame( $transient, $result );
		self::assertObjectNotHasProperty( 'response', $result );
	}

	public function testInjectUpdateAttachesPayloadWhenNewer(): void {
		Functions\when( 'get_site_transient' )->justReturn(
			[
				'version' => '0.2.0',
				'tagName' => 'v0.2.0',
				'htmlUrl' => 'https://github.com/tradik/mddb/releases/tag/v0.2.0',
				'zipUrl'  => 'https://example.com/asset.zip',
				'body'    => 'rel',
				'requiresPhp' => '8.1',
				'tested'  => '6.7',
			]
		);
		$updater = new Updater( 'mddb-sync/mddb-sync.php', '0.1.0', 'tradik/mddb' );
		$result  = $updater->injectUpdate( new \stdClass() );
		self::assertObjectHasProperty( 'response', $result );
		self::assertArrayHasKey( 'mddb-sync/mddb-sync.php', $result->response );
		self::assertSame( '0.2.0', $result->response['mddb-sync/mddb-sync.php']->new_version );
	}

	public function testReleaseNotesAreSanitizedInPluginInformation(): void {
		// INT-003: release notes render as HTML in the wp-admin "View details"
		// modal, so they must pass through wp_kses_post.
		Functions\when( 'get_site_transient' )->justReturn(
			[
				'version'     => '0.2.0',
				'tagName'     => 'v0.2.0',
				'htmlUrl'     => '',
				'zipUrl'      => 'https://example.com/asset.zip',
				'body'        => '<script>alert(document.cookie)</script>Real notes',
				'requiresPhp' => '8.1',
				'tested'      => '6.7',
			]
		);
		$updater = new Updater( 'mddb-sync/mddb-sync.php', '0.1.0', 'tradik/mddb' );
		$info    = $updater->providePluginInformation( false, 'plugin_information', (object) [ 'slug' => 'mddb-sync' ] );

		self::assertIsObject( $info );
		self::assertStringNotContainsString( '<script>', $info->sections['description'] );
		self::assertStringNotContainsString( '<script>', $info->sections['changelog'] );
		self::assertStringContainsString( 'Real notes', $info->sections['description'] );
	}

	public function testInjectUpdateLeavesNonObjectAlone(): void {
		$updater = new Updater( 'mddb-sync/mddb-sync.php', '0.1.0', 'tradik/mddb' );
		self::assertNull( $updater->injectUpdate( null ) );
	}

	public function testLatestReleaseHitsGithubAndCachesResponse(): void {
		Functions\when( 'get_site_transient' )->justReturn( false );
		$captured = null;
		Functions\when( 'set_site_transient' )->alias(
			static function ( $name, $value ) use ( &$captured ) {
				$captured = $value;
				return true;
			}
		);
		Functions\when( 'wp_remote_get' )->justReturn(
			[ 'response' => [ 'code' => 200, 'message' => 'OK' ], 'body' => json_encode( $this->releaseFixture() ) ]
		);

		$updater = new Updater( 'mddb-sync/mddb-sync.php', '0.1.0', 'tradik/mddb' );
		$release = $updater->latestRelease();
		self::assertSame( '0.2.0', $release['version'] );
		self::assertSame( 'https://github.com/tradik/mddb/releases/download/v0.2.0/mddb-sync.zip', $release['zipUrl'] );
		self::assertSame( $captured, $release );
	}

	public function testRejectsUntrustedZipHost(): void {
		// INT-002: a manipulated API response pointing the asset at a hostile
		// host must not become a download_link the WP updater would install.
		Functions\when( 'get_site_transient' )->justReturn( false );
		Functions\when( 'set_site_transient' )->justReturn( true );
		$malicious = $this->releaseFixture();
		$malicious['assets'][0]['browser_download_url'] = 'https://evil.example.com/mddb-sync.zip';
		Functions\when( 'wp_remote_get' )->justReturn(
			[ 'response' => [ 'code' => 200, 'message' => 'OK' ], 'body' => json_encode( $malicious ) ]
		);

		$updater = new Updater( 'mddb-sync/mddb-sync.php', '0.1.0', 'tradik/mddb' );
		$release = $updater->latestRelease();
		self::assertSame( '', $release['zipUrl'], 'untrusted host must yield empty zipUrl (INT-002)' );
	}

	public function testLatestReleaseRecordsErrorOnHttpFailure(): void {
		Functions\when( 'get_site_transient' )->justReturn( false );
		Functions\when( 'wp_remote_get' )->justReturn( new WP_Error( 'net', 'down' ) );
		$cached = null;
		Functions\when( 'set_site_transient' )->alias(
			static function ( $name, $value ) use ( &$cached ) {
				$cached = $value;
				return true;
			}
		);

		$updater = new Updater( 'mddb-sync/mddb-sync.php', '0.1.0', 'tradik/mddb' );
		self::assertNull( $updater->latestRelease() );
		self::assertSame( [ 'error' => 'down' ], $cached );
	}

	public function testProvidePluginInformationReturnsObjectForMatchingSlug(): void {
		Functions\when( 'get_site_transient' )->justReturn(
			[
				'version' => '0.2.0',
				'tagName' => 'v0.2.0',
				'htmlUrl' => 'https://example.com',
				'zipUrl'  => 'https://example.com/asset.zip',
				'body'    => 'changes',
				'requiresPhp' => '8.1',
				'tested'  => '6.7',
			]
		);
		$updater = new Updater( 'mddb-sync/mddb-sync.php', '0.1.0', 'tradik/mddb' );
		$result  = $updater->providePluginInformation( false, 'plugin_information', (object) [ 'slug' => 'mddb-sync' ] );
		self::assertIsObject( $result );
		self::assertSame( 'mddb-sync', $result->slug );
		self::assertSame( '0.2.0', $result->version );
	}

	public function testProvidePluginInformationIgnoresOtherSlug(): void {
		$updater = new Updater( 'mddb-sync/mddb-sync.php', '0.1.0', 'tradik/mddb' );
		self::assertFalse( $updater->providePluginInformation( false, 'plugin_information', (object) [ 'slug' => 'other' ] ) );
	}
}
