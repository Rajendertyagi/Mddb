<?php
/**
 * Assigns languages and links translations via Polylang or WPML.
 *
 * @package Tradik\MddbSync
 */

declare(strict_types=1);

namespace Tradik\MddbSync;

defined( 'ABSPATH' ) || exit;

/**
 * Multilingual glue for remote publishing — the write-side counterpart of
 * {@see Language} (which only reads). Polylang is preferred when both
 * plugins are active, mirroring the read-side auto-detection order.
 *
 * All methods degrade gracefully: when no multilingual plugin is active
 * they return false and the publish flow simply skips language handling.
 */
class Translations {

	private ?bool $forcePolylang;

	private ?bool $forceWpml;

	/**
	 * The nullable flags override plugin detection — they exist for unit
	 * tests, where pll_*()/ICL_* globals cannot be undefined once declared.
	 */
	public function __construct( ?bool $forcePolylang = null, ?bool $forceWpml = null ) {
		$this->forcePolylang = $forcePolylang;
		$this->forceWpml     = $forceWpml;
	}

	/**
	 * Whether any supported multilingual plugin is active.
	 */
	public function isAvailable(): bool {
		return $this->polylangActive() || $this->wpmlActive();
	}

	/**
	 * Assign a language to a post. Accepts locales (`pl_PL`) or slugs (`pl`).
	 */
	public function setLanguage( int $postId, string $lang ): bool {
		$slug = self::slugFrom( $lang );
		if ( $slug === '' ) {
			return false;
		}

		if ( $this->polylangActive() ) {
			pll_set_post_language( $postId, $slug );
			return true;
		}

		if ( $this->wpmlActive() ) {
			// phpcs:disable WordPress.NamingConventions.PrefixAllGlobals.NonPrefixedHooknameFound -- third-party WPML action.
			do_action(
				'wpml_set_element_language_details',
				[
					'element_id'    => $postId,
					'element_type'  => $this->wpmlElementType( $postId ),
					'trid'          => null,
					'language_code' => $slug,
				]
			);
			// phpcs:enable
			return true;
		}

		return false;
	}

	/**
	 * Link a post as the $lang translation of $sourceId.
	 */
	public function link( int $postId, string $lang, int $sourceId ): bool {
		$slug = self::slugFrom( $lang );
		if ( $slug === '' || $sourceId <= 0 ) {
			return false;
		}
		if ( $this->polylangActive() ) {
			return $this->linkViaPolylang( $postId, $slug, $sourceId );
		}
		if ( $this->wpmlActive() ) {
			return $this->linkViaWpml( $postId, $slug, $sourceId );
		}
		return false;
	}

	private function linkViaPolylang( int $postId, string $slug, int $sourceId ): bool {
		pll_set_post_language( $postId, $slug );
		$translations = function_exists( 'pll_get_post_translations' )
			? pll_get_post_translations( $sourceId )
			: [];
		if ( ! is_array( $translations ) ) {
			$translations = [];
		}
		$sourceLang = function_exists( 'pll_get_post_language' )
			? (string) pll_get_post_language( $sourceId, 'slug' )
			: '';
		if ( $sourceLang !== '' ) {
			$translations[ $sourceLang ] = $sourceId;
		}
		$translations[ $slug ] = $postId;
		if ( function_exists( 'pll_save_post_translations' ) ) {
			pll_save_post_translations( $translations );
		}
		return true;
	}

	private function linkViaWpml( int $postId, string $slug, int $sourceId ): bool {
		$elementType = $this->wpmlElementType( $sourceId );
		// phpcs:disable WordPress.NamingConventions.PrefixAllGlobals.NonPrefixedHooknameFound -- third-party WPML filters/actions.
		// apply_filters_ref_array keeps the extra WPML filter arguments out of
		// apply_filters' variadic tail (php:S930 flags the arity mismatch).
		$trid       = apply_filters_ref_array( 'wpml_element_trid', [ null, $sourceId, $elementType ] );
		$sourceLang = apply_filters_ref_array(
			'wpml_element_language_code',
			[
				null,
				[
					'element_id'   => $sourceId,
					'element_type' => $elementType,
				],
			]
		);
		do_action(
			'wpml_set_element_language_details',
			[
				'element_id'           => $postId,
				'element_type'         => $elementType,
				'trid'                 => $trid,
				'language_code'        => $slug,
				'source_language_code' => is_string( $sourceLang ) ? $sourceLang : null,
			]
		);
		// phpcs:enable
		return true;
	}

	/**
	 * `pl_PL` / `pl-PL` / `PL` → `pl` (the slug both plugins key languages by).
	 */
	public static function slugFrom( string $lang ): string {
		$normalised = str_replace( '-', '_', trim( $lang ) );
		$parts      = explode( '_', $normalised, 2 );
		$slug       = strtolower( trim( $parts[0] ) );
		return preg_match( '/^[a-z]{2,3}$/', $slug ) === 1 ? $slug : '';
	}

	private function polylangActive(): bool {
		return $this->forcePolylang ?? function_exists( 'pll_set_post_language' );
	}

	private function wpmlActive(): bool {
		return $this->forceWpml ?? defined( 'ICL_SITEPRESS_VERSION' );
	}

	private function wpmlElementType( int $postId ): string {
		$postType = get_post_type( $postId );
		$postType = is_string( $postType ) && $postType !== '' ? $postType : 'post';
		// phpcs:ignore WordPress.NamingConventions.PrefixAllGlobals.NonPrefixedHooknameFound -- third-party WPML filter.
		$filtered = apply_filters( 'wpml_element_type', $postType );
		return is_string( $filtered ) && $filtered !== '' ? $filtered : 'post_' . $postType;
	}
}
