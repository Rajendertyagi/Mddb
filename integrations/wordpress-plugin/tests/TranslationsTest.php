<?php
/**
 * Tests for the Polylang/WPML publishing glue.
 *
 * @package Tradik\MddbSync\Tests
 */

declare(strict_types=1);

namespace Tradik\MddbSync\Tests;

use Brain\Monkey;
use Brain\Monkey\Functions;
use PHPUnit\Framework\TestCase;
use Tradik\MddbSync\Translations;

final class TranslationsTest extends TestCase {

	protected function setUp(): void {
		parent::setUp();
		Monkey\setUp();
	}

	protected function tearDown(): void {
		Monkey\tearDown();
		parent::tearDown();
	}

	public function testSlugFromNormalisesLocales(): void {
		self::assertSame( 'pl', Translations::slugFrom( 'pl_PL' ) );
		self::assertSame( 'en', Translations::slugFrom( 'en-GB' ) );
		self::assertSame( 'pl', Translations::slugFrom( 'PL' ) );
		self::assertSame( 'pol', Translations::slugFrom( 'pol' ) );
		self::assertSame( '', Translations::slugFrom( '' ) );
		self::assertSame( '', Translations::slugFrom( '123' ) );
	}

	public function testUnavailableWhenNoMultilingualPlugin(): void {
		$translations = new Translations( false, false );
		self::assertFalse( $translations->isAvailable() );
		self::assertFalse( $translations->setLanguage( 5, 'pl_PL' ) );
		self::assertFalse( $translations->link( 5, 'pl_PL', 4 ) );
	}

	public function testSetLanguageRejectsUnparseableCode(): void {
		$translations = new Translations( true, false );
		self::assertFalse( $translations->setLanguage( 5, '!!' ) );
	}

	public function testLinkRejectsMissingSource(): void {
		$translations = new Translations( true, false );
		self::assertFalse( $translations->link( 5, 'pl_PL', 0 ) );
	}

	public function testPolylangSetLanguage(): void {
		Functions\expect( 'pll_set_post_language' )->once()->with( 5, 'pl' );

		$translations = new Translations( true, false );
		self::assertTrue( $translations->setLanguage( 5, 'pl_PL' ) );
	}

	public function testPolylangLinkMergesExistingTranslations(): void {
		Functions\expect( 'pll_set_post_language' )->once()->with( 7, 'pl' );
		Functions\when( 'pll_get_post_translations' )->justReturn( [ 'de' => 3 ] );
		Functions\when( 'pll_get_post_language' )->justReturn( 'en' );
		Functions\expect( 'pll_save_post_translations' )->once()->with(
			[
				'de' => 3,
				'en' => 4,
				'pl' => 7,
			]
		);

		$translations = new Translations( true, false );
		self::assertTrue( $translations->link( 7, 'pl_PL', 4 ) );
	}

	public function testWpmlSetLanguageFiresElementDetails(): void {
		Functions\when( 'get_post_type' )->justReturn( 'page' );
		Functions\expect( 'do_action' )->once()->with(
			'wpml_set_element_language_details',
			\Mockery::on(
				static fn( $args ): bool => is_array( $args )
					&& $args['element_id'] === 9
					&& $args['language_code'] === 'pl'
					&& $args['trid'] === null
			)
		);

		$translations = new Translations( false, true );
		self::assertTrue( $translations->setLanguage( 9, 'pl-PL' ) );
	}

	public function testWpmlLinkPassesTridAndSourceLanguage(): void {
		Functions\when( 'get_post_type' )->justReturn( 'post' );
		Functions\when( 'apply_filters' )->alias(
			static function ( string $hook, $value = null ) {
				if ( $hook === 'wpml_element_trid' ) {
					return 77;
				}
				if ( $hook === 'wpml_element_language_code' ) {
					return 'en';
				}
				return $value;
			}
		);
		Functions\expect( 'do_action' )->once()->with(
			'wpml_set_element_language_details',
			\Mockery::on(
				static fn( $args ): bool => is_array( $args )
					&& $args['element_id'] === 12
					&& $args['trid'] === 77
					&& $args['language_code'] === 'pl'
					&& $args['source_language_code'] === 'en'
			)
		);

		$translations = new Translations( false, true );
		self::assertTrue( $translations->link( 12, 'pl_PL', 4 ) );
	}
}
