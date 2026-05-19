<?php
/**
 * Tests for Settings.
 *
 * @package Tradik\MddbSync\Tests
 */

declare(strict_types=1);

namespace Tradik\MddbSync\Tests;

use Brain\Monkey;
use Brain\Monkey\Functions;
use PHPUnit\Framework\TestCase;
use Tradik\MddbSync\Settings;

final class SettingsTest extends TestCase {

	protected function setUp(): void {
		parent::setUp();
		Monkey\setUp();
		Functions\when( 'home_url' )->justReturn( 'https://example.com' );
		Functions\when( 'get_locale' )->justReturn( 'en_US' );
	}

	protected function tearDown(): void {
		Monkey\tearDown();
		parent::tearDown();
	}

	public function testDefaultsHaveExpectedShape(): void {
		$defaults = Settings::defaults();
		self::assertSame( '', $defaults['url'] );
		self::assertSame( '', $defaults['apiKey'] );
		self::assertSame( 'example_com', $defaults['collection'] );
		self::assertSame( [ 'post', 'page' ], $defaults['postTypes'] );
		self::assertTrue( $defaults['syncOnSave'] );
		self::assertTrue( $defaults['syncOnDelete'] );
		self::assertSame( Settings::LANG_AUTO, $defaults['languageSource'] );
		self::assertSame( Settings::KEY_POST_ID, $defaults['keyStrategy'] );
		self::assertFalse( $defaults['includeDrafts'] );
	}

	public function testCollectionDefaultsToWordpressForEmptyHost(): void {
		Functions\when( 'home_url' )->justReturn( '' );
		self::assertSame( 'wordpress', Settings::defaults()['collection'] );
	}

	public function testReadsTypedAccessorsWithFallbacks(): void {
		Functions\when( 'get_option' )->justReturn(
			[
				'url'    => 'https://mddb.example.com',
				'apiKey' => 'vk_secret',
			]
		);
		$settings = new Settings();
		self::assertSame( 'https://mddb.example.com', $settings->url() );
		self::assertSame( 'vk_secret', $settings->apiKey() );
		self::assertTrue( $settings->isConfigured() );
		self::assertSame( 'example_com', $settings->collection() );
		self::assertSame( [ 'post', 'page' ], $settings->postTypes() );
	}

	public function testIsConfiguredReturnsFalseWhenUrlMissing(): void {
		Functions\when( 'get_option' )->justReturn( [] );
		$settings = new Settings();
		self::assertFalse( $settings->isConfigured() );
	}

	public function testInvalidStrategyFallsBackToDefault(): void {
		Functions\when( 'get_option' )->justReturn(
			[
				'keyStrategy'    => 'something-bogus',
				'languageSource' => 'klingon',
			]
		);
		$settings = new Settings();
		self::assertSame( Settings::KEY_POST_ID, $settings->keyStrategy() );
		self::assertSame( Settings::LANG_AUTO, $settings->languageSource() );
	}

	public function testSanitizeAcceptsValidInput(): void {
		Functions\when( 'wp_http_validate_url' )->alias(
			static fn( $url ) => filter_var( $url, FILTER_VALIDATE_URL ) !== false
		);
		Functions\when( 'esc_url_raw' )->returnArg();
		Functions\when( 'untrailingslashit' )->alias( static fn( $v ) => rtrim( (string) $v, '/' ) );
		Functions\when( 'sanitize_key' )->alias(
			static fn( $v ) => preg_replace( '/[^a-z0-9_\-]/', '', strtolower( (string) $v ) )
		);

		$out = Settings::sanitize(
			[
				'url'            => 'https://mddb.example.com/',
				'apiKey'         => '  vk_xyz  ',
				'collection'     => 'My-Collection!',
				'postTypes'      => [ 'post', 'page', 'BAD!' ],
				'syncOnSave'     => '1',
				'syncOnDelete'   => '',
				'includeDrafts'  => '1',
				'languageSource' => Settings::LANG_POLYLANG,
				'keyStrategy'    => Settings::KEY_PERMALINK,
			]
		);

		self::assertSame( 'https://mddb.example.com', $out['url'] );
		self::assertSame( 'vk_xyz', $out['apiKey'] );
		self::assertSame( 'my-collection', $out['collection'] );
		self::assertSame( [ 'post', 'page', 'bad' ], $out['postTypes'] );
		self::assertTrue( $out['syncOnSave'] );
		self::assertFalse( $out['syncOnDelete'] );
		self::assertTrue( $out['includeDrafts'] );
		self::assertSame( Settings::LANG_POLYLANG, $out['languageSource'] );
		self::assertSame( Settings::KEY_PERMALINK, $out['keyStrategy'] );
	}

	public function testSanitizeRejectsInvalidUrlAndStrategy(): void {
		Functions\when( 'wp_http_validate_url' )->justReturn( false );
		Functions\when( 'sanitize_key' )->alias(
			static fn( $v ) => preg_replace( '/[^a-z0-9_\-]/', '', strtolower( (string) $v ) )
		);
		$out = Settings::sanitize(
			[
				'url'            => 'javascript:alert(1)',
				'languageSource' => 'sumerian',
				'keyStrategy'    => 'phoenician',
			]
		);
		self::assertSame( '', $out['url'] );
		self::assertSame( Settings::LANG_AUTO, $out['languageSource'] );
		self::assertSame( Settings::KEY_POST_ID, $out['keyStrategy'] );
	}
}
