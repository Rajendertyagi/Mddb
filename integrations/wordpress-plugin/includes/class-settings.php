<?php
/**
 * Typed access to the plugin's option array.
 *
 * @package Tradik\MddbSync
 */

declare(strict_types=1);

namespace Tradik\MddbSync;

defined( 'ABSPATH' ) || exit;

/**
 * Persisted plugin configuration with safe accessors and defaults.
 *
 * Storage layout (single autoloaded option `mddb_sync_options`):
 *   url, apiKey, collection, postTypes[], syncOnSave, syncOnDelete,
 *   languageSource, keyStrategy, includeDrafts.
 */
final class Settings {

	public const OPTION_NAME = Plugin::OPTION_NAME;

	public const LANG_AUTO     = 'auto';
	public const LANG_POLYLANG = 'polylang';
	public const LANG_WPML     = 'wpml';
	public const LANG_LOCALE   = 'locale';

	public const KEY_POST_ID   = 'postId';
	public const KEY_POST_SLUG = 'postSlug';
	public const KEY_PERMALINK = 'permalink';

	/**
	 * Default values shipped on first activation.
	 *
	 * @return array<string,mixed>
	 */
	public static function defaults(): array {
		return [
			'url'            => '',
			'apiKey'         => '',
			'collection'     => self::deriveCollectionFromHost(),
			'postTypes'      => [ 'post', 'page' ],
			'syncOnSave'     => true,
			'syncOnDelete'   => true,
			'languageSource' => self::LANG_AUTO,
			'keyStrategy'    => self::KEY_POST_ID,
			'includeDrafts'  => false,
		];
	}

	/**
	 * @return array<string,mixed>
	 */
	public function all(): array {
		$saved = get_option( self::OPTION_NAME, [] );
		if ( ! is_array( $saved ) ) {
			$saved = [];
		}
		return array_replace( self::defaults(), $saved );
	}

	public function url(): string {
		return (string) ( $this->all()['url'] ?? '' );
	}

	public function apiKey(): string {
		return (string) ( $this->all()['apiKey'] ?? '' );
	}

	public function collection(): string {
		$value = (string) ( $this->all()['collection'] ?? '' );
		return $value !== '' ? $value : self::deriveCollectionFromHost();
	}

	/**
	 * @return string[]
	 */
	public function postTypes(): array {
		$raw = $this->all()['postTypes'] ?? [];
		if ( ! is_array( $raw ) ) {
			return [ 'post', 'page' ];
		}
		return array_values(
			array_filter(
				array_map( 'strval', $raw ),
				static fn( string $type ): bool => $type !== ''
			)
		);
	}

	public function syncOnSave(): bool {
		return (bool) ( $this->all()['syncOnSave'] ?? true );
	}

	public function syncOnDelete(): bool {
		return (bool) ( $this->all()['syncOnDelete'] ?? true );
	}

	public function includeDrafts(): bool {
		return (bool) ( $this->all()['includeDrafts'] ?? false );
	}

	public function languageSource(): string {
		$value = (string) ( $this->all()['languageSource'] ?? self::LANG_AUTO );
		return in_array(
			$value,
			[ self::LANG_AUTO, self::LANG_POLYLANG, self::LANG_WPML, self::LANG_LOCALE ],
			true
		) ? $value : self::LANG_AUTO;
	}

	public function keyStrategy(): string {
		$value = (string) ( $this->all()['keyStrategy'] ?? self::KEY_POST_ID );
		return in_array(
			$value,
			[ self::KEY_POST_ID, self::KEY_POST_SLUG, self::KEY_PERMALINK ],
			true
		) ? $value : self::KEY_POST_ID;
	}

	public function isConfigured(): bool {
		return $this->url() !== '';
	}

	/**
	 * @param array<string,mixed> $input
	 * @return array<string,mixed>
	 */
	public static function sanitize( array $input ): array {
		$out = self::defaults();

		if ( isset( $input['url'] ) ) {
			$url = trim( (string) $input['url'] );
			if ( $url !== '' && wp_http_validate_url( $url ) ) {
				$out['url'] = untrailingslashit( esc_url_raw( $url ) );
			}
		}

		if ( isset( $input['apiKey'] ) ) {
			$out['apiKey'] = trim( (string) $input['apiKey'] );
		}

		if ( isset( $input['collection'] ) ) {
			$collection = sanitize_key( (string) $input['collection'] );
			if ( $collection !== '' ) {
				$out['collection'] = $collection;
			}
		}

		if ( isset( $input['postTypes'] ) && is_array( $input['postTypes'] ) ) {
			$out['postTypes'] = array_values(
				array_filter(
					array_map( 'sanitize_key', $input['postTypes'] ),
					static fn( string $type ): bool => $type !== ''
				)
			);
		}

		$out['syncOnSave']    = ! empty( $input['syncOnSave'] );
		$out['syncOnDelete']  = ! empty( $input['syncOnDelete'] );
		$out['includeDrafts'] = ! empty( $input['includeDrafts'] );

		if ( isset( $input['languageSource'] ) ) {
			$value = (string) $input['languageSource'];
			if ( in_array(
				$value,
				[ self::LANG_AUTO, self::LANG_POLYLANG, self::LANG_WPML, self::LANG_LOCALE ],
				true
			) ) {
				$out['languageSource'] = $value;
			}
		}

		if ( isset( $input['keyStrategy'] ) ) {
			$value = (string) $input['keyStrategy'];
			if ( in_array(
				$value,
				[ self::KEY_POST_ID, self::KEY_POST_SLUG, self::KEY_PERMALINK ],
				true
			) ) {
				$out['keyStrategy'] = $value;
			}
		}

		return $out;
	}

	private static function deriveCollectionFromHost(): string {
		$host = '';
		if ( function_exists( 'wp_parse_url' ) ) {
			$home = function_exists( 'home_url' ) ? home_url() : '';
			$host = (string) wp_parse_url( $home, PHP_URL_HOST );
		}
		if ( $host === '' ) {
			return 'wordpress';
		}
		$slug = preg_replace( '/[^a-z0-9]+/i', '_', strtolower( $host ) );
		return trim( (string) $slug, '_' ) !== '' ? trim( (string) $slug, '_' ) : 'wordpress';
	}
}
