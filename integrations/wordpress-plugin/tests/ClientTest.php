<?php
/**
 * Tests for the MDDB HTTP client.
 *
 * @package Tradik\MddbSync\Tests
 */

declare(strict_types=1);

namespace Tradik\MddbSync\Tests;

use Brain\Monkey;
use Brain\Monkey\Functions;
use PHPUnit\Framework\TestCase;
use Tradik\MddbSync\Client;
use Tradik\MddbSync\Settings;
use WP_Error;

final class ClientTest extends TestCase {

	protected function setUp(): void {
		parent::setUp();
		Monkey\setUp();
		Functions\when( 'wp_json_encode' )->alias( static fn( $v ) => json_encode( $v ) );
		Functions\when( 'home_url' )->justReturn( 'https://example.com' );
	}

	protected function tearDown(): void {
		Monkey\tearDown();
		parent::tearDown();
	}

	private function settingsWith( string $url, string $key = '' ): Settings {
		Functions\when( 'get_option' )->justReturn(
			[ 'url' => $url, 'apiKey' => $key, 'collection' => 'c' ]
		);
		Functions\when( 'get_locale' )->justReturn( 'en_US' );
		return new Settings();
	}

	public function testPingReturnsFalseWhenNotConfigured(): void {
		$client = new Client( $this->settingsWith( '' ) );
		$result = $client->ping();
		self::assertFalse( $result['ok'] );
	}

	public function testPingReportsSuccessOn2xx(): void {
		Functions\when( 'wp_remote_post' )->justReturn(
			[ 'response' => [ 'code' => 200, 'message' => 'OK' ], 'body' => '{}' ]
		);
		$client = new Client( $this->settingsWith( 'https://mddb.example.com', 'vk_x' ) );
		$result = $client->ping();
		self::assertTrue( $result['ok'] );
		self::assertSame( 200, $result['status'] );
	}

	public function testPingDetectsAuthFailure(): void {
		Functions\when( 'wp_remote_post' )->justReturn(
			[ 'response' => [ 'code' => 403, 'message' => 'Forbidden' ], 'body' => '{}' ]
		);
		$client = new Client( $this->settingsWith( 'https://mddb.example.com' ) );
		$result = $client->ping();
		self::assertFalse( $result['ok'] );
		self::assertSame( 403, $result['status'] );
	}

	public function testPingDetects5xx(): void {
		Functions\when( 'wp_remote_post' )->justReturn(
			[ 'response' => [ 'code' => 503, 'message' => 'unavailable' ], 'body' => '{}' ]
		);
		$client = new Client( $this->settingsWith( 'https://mddb.example.com' ) );
		$result = $client->ping();
		self::assertFalse( $result['ok'] );
		self::assertSame( 503, $result['status'] );
	}

	public function testPingPropagatesWpError(): void {
		Functions\when( 'wp_remote_post' )->justReturn( new WP_Error( 'http', 'down' ) );
		$client = new Client( $this->settingsWith( 'https://mddb.example.com' ) );
		$result = $client->ping();
		self::assertFalse( $result['ok'] );
		self::assertSame( 'down', $result['message'] );
	}

	public function testAddDocumentSuccess(): void {
		Functions\when( 'wp_remote_post' )->justReturn(
			[ 'response' => [ 'code' => 200, 'message' => 'OK' ], 'body' => '{}' ]
		);
		$client = new Client( $this->settingsWith( 'https://mddb.example.com', 'vk_x' ) );
		self::assertTrue( $client->addDocument( [ 'collection' => 'c', 'key' => 'k', 'lang' => 'en' ] ) );
	}

	public function testAddDocumentReturnsWpErrorOnNon2xx(): void {
		Functions\when( 'wp_remote_post' )->justReturn(
			[ 'response' => [ 'code' => 400, 'message' => 'bad' ], 'body' => '{}' ]
		);
		$client  = new Client( $this->settingsWith( 'https://mddb.example.com' ) );
		$result  = $client->addDocument( [ 'collection' => 'c', 'key' => 'k', 'lang' => 'en' ] );
		self::assertInstanceOf( WP_Error::class, $result );
	}

	public function testAddDocumentRetriesTransientErrors(): void {
		$responses = [
			[ 'response' => [ 'code' => 503, 'message' => 'unavailable' ], 'body' => '' ],
			[ 'response' => [ 'code' => 500, 'message' => 'oops' ], 'body' => '' ],
			[ 'response' => [ 'code' => 200, 'message' => 'OK' ], 'body' => '{}' ],
		];
		$calls = 0;
		Functions\when( 'wp_remote_post' )->alias(
			static function () use ( &$calls, &$responses ) {
				$value = $responses[ $calls ] ?? $responses[ count( $responses ) - 1 ];
				$calls++;
				return $value;
			}
		);

		$client = new Client( $this->settingsWith( 'https://mddb.example.com' ) );
		self::assertTrue( $client->addDocument( [ 'collection' => 'c', 'key' => 'k', 'lang' => 'en' ] ) );
		self::assertSame( 3, $calls );
	}

	public function testDeleteDocumentTreats404AsSuccess(): void {
		Functions\when( 'wp_remote_post' )->justReturn(
			[ 'response' => [ 'code' => 404, 'message' => 'gone' ], 'body' => '' ]
		);
		$client = new Client( $this->settingsWith( 'https://mddb.example.com' ) );
		self::assertTrue( $client->deleteDocument( 'c', 'k', 'en' ) );
	}

	public function testDeleteDocumentReturnsWpErrorOn500(): void {
		Functions\when( 'wp_remote_post' )->justReturn(
			[ 'response' => [ 'code' => 500, 'message' => 'oops' ], 'body' => 'oops' ]
		);
		$client = new Client( $this->settingsWith( 'https://mddb.example.com' ) );
		$result = $client->deleteDocument( 'c', 'k', 'en' );
		self::assertInstanceOf( WP_Error::class, $result );
	}

	public function testAddDocumentErrorsWhenUrlMissing(): void {
		$client = new Client( $this->settingsWith( '' ) );
		$result = $client->addDocument( [ 'collection' => 'c', 'key' => 'k', 'lang' => 'en' ] );
		self::assertInstanceOf( WP_Error::class, $result );
	}
}
