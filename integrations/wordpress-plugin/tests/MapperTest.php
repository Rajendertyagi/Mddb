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
		Functions\when( 'get_object_taxonomies' )->justReturn( [ 'category', 'post_tag' ] );
		Functions\when( 'get_post_meta' )->justReturn( [] );
		Functions\when( 'get_fields' )->justReturn( [] );
		Functions\when( 'wp_json_encode' )->alias( static fn( $v ) => json_encode( $v ) );
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

	public function testFullMetaCaptureFromPostMetaTaxonomiesAcfAndSeo(): void {
		Functions\when( 'get_object_taxonomies' )->justReturn( [ 'category', 'project_type' ] );
		Functions\when( 'get_the_terms' )->alias(
			static function ( $post, $tax ) {
				if ( $tax === 'category' ) {
					return [ new \WP_Term( [ 'name' => 'News', 'term_id' => 1 ] ) ];
				}
				if ( $tax === 'project_type' ) {
					return [ new \WP_Term( [ 'name' => 'Internal', 'term_id' => 7 ] ) ];
				}
				return [];
			}
		);
		Functions\when( 'get_post_meta' )->justReturn(
			[
				'_thumbnail_id'           => [ '42' ],
				'_yoast_wpseo_title'      => [ 'Yoast title' ],
				'_yoast_wpseo_metadesc'   => [ 'Yoast desc' ],
				'custom_field'            => [ 'foo', 'bar' ],
				'serialized_array_field'  => [ [ 'a' => 1, 'b' => [ 2, 3 ] ] ],
			]
		);
		Functions\when( 'get_fields' )->justReturn(
			[
				'hero_text'  => 'Welcome!',
				'gallery_id' => 17,
			]
		);
		Functions\when( 'apply_filters' )->alias(
			static function ( $hook, $value ) {
				if ( $hook === 'the_content' ) { return '<p>body</p>'; }
				if ( $hook === 'mddb_sync_meta' ) { return $value; }
				return $value;
			}
		);

		$post = new \WP_Post(
			[ 'ID' => 12, 'post_title' => 'T', 'post_type' => 'post', 'post_status' => 'publish' ]
		);
		$mapper = new Mapper( new Language() );
		$doc    = $mapper->toDocument( $post, 'site', Settings::KEY_POST_ID, Settings::LANG_LOCALE );

		// Built-in keys still ship.
		self::assertSame( [ 'post' ], $doc['meta']['postType'] );
		// Custom taxonomy lands under its own slug; legacy `category` aliased to `categories`.
		self::assertSame( [ 'News' ], $doc['meta']['categories'] );
		self::assertSame( [ 'Internal' ], $doc['meta']['project_type'] );
		// Raw post meta + ACF.
		self::assertSame( [ '42' ], $doc['meta']['_thumbnail_id'] );
		self::assertSame( [ 'foo', 'bar' ], $doc['meta']['custom_field'] );
		self::assertSame( [ 'Welcome!' ], $doc['meta']['acf:hero_text'] );
		self::assertSame( [ '17' ], $doc['meta']['acf:gallery_id'] );
		// SEO normalised on top of the raw `_yoast_wpseo_*` keys.
		self::assertSame( [ 'Yoast title' ], $doc['meta']['seoTitle'] );
		self::assertSame( [ 'Yoast desc' ], $doc['meta']['seoDescription'] );
		self::assertSame( [ 'yoast' ], $doc['meta']['seoSource'] );
		// Array meta values JSON-encoded.
		self::assertSame( [ json_encode( [ 'a' => 1, 'b' => [ 2, 3 ] ] ) ], $doc['meta']['serialized_array_field'] );
	}

	public function testMddbSyncMetaFilterCanOverrideOutput(): void {
		Functions\when( 'get_object_taxonomies' )->justReturn( [] );
		Functions\when( 'get_post_meta' )->justReturn( [ '_edit_lock' => [ '123:5' ] ] );
		Functions\when( 'apply_filters' )->alias(
			static function ( $hook, $value ) {
				if ( $hook === 'mddb_sync_meta' ) {
					unset( $value['_edit_lock'] );
					$value['custom'] = [ 'injected' ];
					return $value;
				}
				if ( $hook === 'the_content' ) { return ''; }
				return $value;
			}
		);
		$post   = new \WP_Post( [ 'ID' => 1, 'post_type' => 'post' ] );
		$mapper = new Mapper( new Language() );
		$doc    = $mapper->toDocument( $post, 'c', Settings::KEY_POST_ID, Settings::LANG_LOCALE );
		self::assertArrayNotHasKey( '_edit_lock', $doc['meta'] );
		self::assertSame( [ 'injected' ], $doc['meta']['custom'] );
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
