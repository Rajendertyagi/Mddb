<?php
/**
 * Tests for the SEO normaliser.
 *
 * @package Tradik\MddbSync\Tests
 */

declare(strict_types=1);

namespace Tradik\MddbSync\Tests;

use Brain\Monkey;
use Brain\Monkey\Functions;
use PHPUnit\Framework\TestCase;
use Tradik\MddbSync\Seo;

final class SeoTest extends TestCase {

	protected function setUp(): void {
		parent::setUp();
		Monkey\setUp();
		Functions\when( 'wp_json_encode' )->alias( static fn( $v ) => json_encode( $v ) );
	}

	protected function tearDown(): void {
		Monkey\tearDown();
		parent::tearDown();
	}

	public function testEmptyMetaProducesEmptyExtract(): void {
		self::assertSame( [], ( new Seo() )->extract( [] ) );
	}

	public function testYoastMetaExtractedAndTagged(): void {
		$meta = [
			'_yoast_wpseo_title'      => [ 'Yoast title' ],
			'_yoast_wpseo_metadesc'   => [ 'Yoast desc' ],
			'_yoast_wpseo_focuskw'    => [ 'keyword' ],
		];
		$out  = ( new Seo() )->extract( $meta );
		self::assertSame( [ 'yoast' ], $out['seoSource'] );
		self::assertSame( [ 'Yoast title' ], $out['seoTitle'] );
		self::assertSame( [ 'Yoast desc' ], $out['seoDescription'] );
		self::assertSame( [ 'keyword' ], $out['seoFocusKeyword'] );
	}

	public function testRankMathWinsWhenItHasMoreFields(): void {
		$meta = [
			'_yoast_wpseo_title'   => [ 'Yoast only' ],
			'rank_math_title'      => [ 'RM title' ],
			'rank_math_description' => [ 'RM desc' ],
			'rank_math_canonical_url' => [ 'https://example.com/canonical' ],
		];
		$out = ( new Seo() )->extract( $meta );
		self::assertSame( [ 'rankmath' ], $out['seoSource'] );
		self::assertSame( [ 'RM title' ], $out['seoTitle'] );
		self::assertSame( [ 'https://example.com/canonical' ], $out['seoCanonical'] );
	}

	public function testSeoPressPicks(): void {
		$meta = [
			'_seopress_titles_title' => [ 'SEOPress title' ],
			'_seopress_titles_desc'  => [ 'SEOPress desc' ],
			'_seopress_robots_canonical' => [ 'https://example.com/x' ],
		];
		$out = ( new Seo() )->extract( $meta );
		self::assertSame( [ 'seopress' ], $out['seoSource'] );
		self::assertSame( [ 'SEOPress title' ], $out['seoTitle'] );
	}

	public function testArrayValuesAreJsonEncoded(): void {
		$meta = [
			'_yoast_wpseo_opengraph-image' => [ [ 'url' => 'https://example.com/a.png', 'id' => 4 ] ],
		];
		$out = ( new Seo() )->extract( $meta );
		self::assertSame( [ json_encode( [ 'url' => 'https://example.com/a.png', 'id' => 4 ] ) ], $out['ogImage'] );
	}

	public function testBooleansStringified(): void {
		$meta = [
			'_yoast_wpseo_meta-robots-noindex' => [ true ],
		];
		$out = ( new Seo() )->extract( $meta );
		self::assertSame( [ '1' ], $out['seoRobotsNoindex'] );
	}
}
