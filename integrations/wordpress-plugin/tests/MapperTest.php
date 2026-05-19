<?php
/**
 * Tests for Mapper (WP_Post → MDDB document).
 *
 * @package Tradik\MddbSync\Tests
 */

declare(strict_types=1);

namespace Tradik\MddbSync\Tests;

use Brain\Monkey;
use Brain\Monkey\Functions;
use PHPUnit\Framework\TestCase;
use Tradik\MddbSync\Language;
use Tradik\MddbSync\Mapper;
use Tradik\MddbSync\Settings;
use WP_Post;

final class MapperTest extends TestCase {

	protected function setUp(): void {
		parent::setUp();
		Monkey\setUp();
		Functions\when( 'get_locale' )->justReturn( 'en_US' );
		Functions\when( 'apply_filters' )->returnArg( 2 );
		Functions\when( 'get_post_type' )->justReturn( 'post' );
		Functions\when( 'sanitize_key' )->alias(
			static fn( $v ) => (string) preg_replace( '/[^a-z0-9_\-]/', '', strtolower( (string) $v ) )
		);
		Functions\when( 'sanitize_title' )->alias(
			static fn( $v ) => trim( (string) preg_replace( '/[^a-z0-9]+/', '-', strtolower( (string) $v ) ), '-' )
		);
		Functions\when( 'get_post_time' )->justReturn( '2026-05-19T00:00:00+00:00' );
		Functions\when( 'get_post_modified_time' )->justReturn( '2026-05-19T00:00:00+00:00' );
		Functions\when( 'get_the_author_meta' )->justReturn( 'Jane Author' );
		Functions\when( 'get_permalink' )->justReturn( 'https://example.com/2026/05/hello-world/' );
		Functions\when( 'get_the_terms' )->justReturn( [] );
	}

	protected function tearDown(): void {
		Monkey\tearDown();
		parent::tearDown();
	}

	public function testKeyByPostId(): void {
		$post   = new WP_Post( [ 'ID' => 42, 'post_type' => 'page' ] );
		$mapper = new Mapper( new Language() );
		self::assertSame( 'page-42', $mapper->keyFor( $post, Settings::KEY_POST_ID ) );
	}

	public function testKeyByPostSlugFallsBackToIdWhenEmpty(): void {
		$post   = new WP_Post( [ 'ID' => 7, 'post_type' => 'post', 'post_name' => '' ] );
		$mapper = new Mapper( new Language() );
		self::assertSame( 'post-7', $mapper->keyFor( $post, Settings::KEY_POST_SLUG ) );
	}

	public function testKeyByPermalinkReturnsPath(): void {
		$post   = new WP_Post( [ 'ID' => 9, 'post_type' => 'post' ] );
		$mapper = new Mapper( new Language() );
		self::assertSame( '2026/05/hello-world', $mapper->keyFor( $post, Settings::KEY_PERMALINK ) );
	}

	public function testKeyByPermalinkFallsBackWhenPermalinkEmpty(): void {
		Functions\when( 'get_permalink' )->justReturn( '' );
		$post   = new WP_Post( [ 'ID' => 99, 'post_type' => 'page' ] );
		$mapper = new Mapper( new Language() );
		self::assertSame( 'page-99', $mapper->keyFor( $post, Settings::KEY_PERMALINK ) );
	}

	public function testToDocumentBuildsExpectedPayload(): void {
		Functions\when( 'apply_filters' )->alias(
			static function ( $hook, $value ) {
				if ( $hook === 'the_content' ) {
					return '<p>Hello <strong>world</strong>.</p>';
				}
				return $value;
			}
		);
		Functions\when( 'get_the_terms' )->alias(
			static function ( $post, $tax ) {
				if ( $tax === 'category' ) {
					return [ new \WP_Term( [ 'name' => 'News' ] ) ];
				}
				return [];
			}
		);

		$post = new WP_Post(
			[
				'ID'           => 12,
				'post_title'   => 'Hello world',
				'post_name'    => 'hello-world',
				'post_type'    => 'post',
				'post_status'  => 'publish',
				'post_excerpt' => 'short',
				'post_author'  => 3,
			]
		);

		$mapper = new Mapper( new Language() );
		$doc    = $mapper->toDocument( $post, 'site_x', Settings::KEY_POST_ID, Settings::LANG_LOCALE );

		self::assertSame( 'site_x', $doc['collection'] );
		self::assertSame( 'post-12', $doc['key'] );
		self::assertSame( 'en_US', $doc['lang'] );
		self::assertSame( [ 'post' ], $doc['meta']['postType'] );
		self::assertSame( [ 'Hello world' ], $doc['meta']['title'] );
		self::assertSame( [ 'News' ], $doc['meta']['categories'] );
		self::assertSame( [ 'Jane Author' ], $doc['meta']['author'] );
		self::assertSame( [ 'short' ], $doc['meta']['excerpt'] );
		self::assertStringContainsString( '# Hello world', $doc['contentMd'] );
		self::assertStringContainsString( 'Hello world.', $doc['contentMd'] );
	}
}
