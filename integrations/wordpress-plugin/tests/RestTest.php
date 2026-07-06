<?php
/**
 * Tests for the mddb-sync/v1 REST controller.
 *
 * @package Tradik\MddbSync\Tests
 */

declare(strict_types=1);

namespace Tradik\MddbSync\Tests;

use Brain\Monkey;
use Brain\Monkey\Functions;
use Mockery;
use PHPUnit\Framework\TestCase;
use Tradik\MddbSync\Publisher;
use Tradik\MddbSync\Rest;
use Tradik\MddbSync\Settings;
use WP_Error;
use WP_REST_Request;

final class RestTest extends TestCase {

	/** @var Publisher&Mockery\MockInterface */
	private $publisher;

	protected function setUp(): void {
		parent::setUp();
		Monkey\setUp();
		Functions\when( 'home_url' )->justReturn( 'https://example.com' );
		Functions\when( 'rest_ensure_response' )->returnArg();
		$this->publisher = Mockery::mock( Publisher::class );
	}

	protected function tearDown(): void {
		Monkey\tearDown();
		Mockery::close();
		parent::tearDown();
	}

	private function rest( bool $enabled = true, string $key = 'vk_secret' ): Rest {
		Functions\when( 'get_option' )->justReturn(
			[
				'enablePublish' => $enabled,
				'publishKey'    => $key,
			]
		);
		return new Rest( new Settings(), $this->publisher );
	}

	public function testRegisterHooksRestApiInit(): void {
		$hooked = [];
		Functions\when( 'add_action' )->alias(
			static function ( string $hook, $callback ) use ( &$hooked ): bool {
				$hooked[] = [ $hook, $callback ];
				return true;
			}
		);

		$this->rest()->register();

		self::assertCount( 1, $hooked );
		self::assertSame( 'rest_api_init', $hooked[0][0] );
		self::assertSame( 'registerRoutes', $hooked[0][1][1] );
	}

	public function testRegisterRoutesRegistersBothEndpoints(): void {
		$routes = [];
		Functions\when( 'register_rest_route' )->alias(
			static function ( string $ns, string $route, array $args ) use ( &$routes ): bool {
				$routes[ $route ] = [ $ns, $args ];
				return true;
			}
		);

		$this->rest()->registerRoutes();

		self::assertSame( [ '/publish', '/status' ], array_keys( $routes ) );
		foreach ( $routes as $route ) {
			self::assertSame( 'mddb-sync/v1', $route[0] );
			self::assertSame( 'POST', $route[1]['methods'] );
			self::assertIsCallable( $route[1]['permission_callback'] );
		}
	}

	public function testAuthRejectedWhenDisabled(): void {
		$result = $this->rest( false )->checkAuth( new WP_REST_Request() );
		self::assertInstanceOf( WP_Error::class, $result );
		self::assertSame( 'mddb_publish_disabled', $result->get_error_code() );
	}

	public function testAuthRejectedWithoutConfiguredKey(): void {
		$result = $this->rest( true, '' )->checkAuth( new WP_REST_Request() );
		self::assertInstanceOf( WP_Error::class, $result );
		self::assertSame( 'mddb_publish_no_key', $result->get_error_code() );
	}

	public function testAuthRejectedOnWrongKey(): void {
		$request = new WP_REST_Request( [ 'Authorization' => 'Bearer nope' ] );
		$result  = $this->rest()->checkAuth( $request );
		self::assertInstanceOf( WP_Error::class, $result );
		self::assertSame( 'mddb_publish_unauthorized', $result->get_error_code() );
	}

	public function testAuthRejectedWithoutAnyHeader(): void {
		$result = $this->rest()->checkAuth( new WP_REST_Request() );
		self::assertInstanceOf( WP_Error::class, $result );
		self::assertSame( 'mddb_publish_unauthorized', $result->get_error_code() );
	}

	public function testAuthAcceptsBearerHeader(): void {
		$request = new WP_REST_Request( [ 'Authorization' => 'Bearer vk_secret' ] );
		self::assertTrue( $this->rest()->checkAuth( $request ) );
	}

	public function testAuthAcceptsCustomHeader(): void {
		$request = new WP_REST_Request( [ 'X-MDDB-Publish-Key' => 'vk_secret' ] );
		self::assertTrue( $this->rest()->checkAuth( $request ) );
	}

	public function testPublishRejectsNonObjectBody(): void {
		$result = $this->rest()->handlePublish( new WP_REST_Request( [], 'not-json' ) );
		self::assertInstanceOf( WP_Error::class, $result );
		self::assertSame( 'mddb_publish_bad_payload', $result->get_error_code() );
	}

	public function testPublishDelegatesToPublisher(): void {
		$payload  = [ 'title' => 'Hello', 'status' => 'publish' ];
		$expected = [ 'id' => 1, 'created' => true ];
		$this->publisher->shouldReceive( 'publish' )->once()->with( $payload )->andReturn( $expected );

		$result = $this->rest()->handlePublish( new WP_REST_Request( [], $payload ) );

		self::assertSame( $expected, $result );
	}

	public function testPublishPropagatesPublisherError(): void {
		$error = new WP_Error( 'mddb_publish_bad_type', 'nope', [ 'status' => 400 ] );
		$this->publisher->shouldReceive( 'publish' )->once()->andReturn( $error );

		$result = $this->rest()->handlePublish( new WP_REST_Request( [], [ 'type' => 'x' ] ) );

		self::assertSame( $error, $result );
	}

	public function testStatusDelegatesToPublisher(): void {
		$payload  = [ 'id' => 4, 'status' => 'draft' ];
		$expected = [ 'id' => 4, 'status' => 'draft' ];
		$this->publisher->shouldReceive( 'changeStatus' )->once()->with( $payload )->andReturn( $expected );

		$result = $this->rest()->handleStatus( new WP_REST_Request( [], $payload ) );

		self::assertSame( $expected, $result );
	}

	public function testStatusRejectsEmptyBody(): void {
		$result = $this->rest()->handleStatus( new WP_REST_Request( [], [] ) );
		self::assertInstanceOf( WP_Error::class, $result );
		self::assertSame( 'mddb_publish_bad_payload', $result->get_error_code() );
	}
}
