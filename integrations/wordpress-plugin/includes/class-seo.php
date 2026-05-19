<?php
/**
 * Normalises SEO metadata across Yoast / RankMath / SEOPress.
 *
 * @package Tradik\MddbSync
 */

declare(strict_types=1);

namespace Tradik\MddbSync;

defined( 'ABSPATH' ) || exit;

/**
 * Extracts a unified SEO bag from whatever SEO plugin is installed.
 *
 * Output keys (all optional, only included when the source plugin provides them):
 *   seoSource, seoTitle, seoDescription, seoFocusKeyword, seoCanonical, seoRobots,
 *   ogTitle, ogDescription, ogImage, twitterTitle, twitterDescription, twitterImage.
 *
 * Raw `_yoast_wpseo_*` / `rank_math_*` / `_seopress_*` post-meta still ships in
 * the main meta bag — these normalised fields just give MDDB consumers a stable
 * key-set without caring which plugin wrote the data.
 */
final class Seo {

	/**
	 * @param array<string,array<int,mixed>> $postMeta Raw `get_post_meta($id, '', false)` output.
	 * @return array<string,array<int,string>>
	 */
	public function extract( array $postMeta ): array {
		$yoast    = $this->extractFromYoast( $postMeta );
		$rankmath = $this->extractFromRankMath( $postMeta );
		$seopress = $this->extractFromSeoPress( $postMeta );

		// Pick the source with the most fields filled in (Yoast wins ties for stability).
		$best = $yoast;
		if ( count( $rankmath ) > count( $best ) ) {
			$best = $rankmath;
		}
		if ( count( $seopress ) > count( $best ) ) {
			$best = $seopress;
		}
		return $best;
	}

	/**
	 * @param array<string,array<int,mixed>> $postMeta
	 * @return array<string,array<int,string>>
	 */
	private function extractFromYoast( array $postMeta ): array {
		$mapping = [
			'_yoast_wpseo_title'             => 'seoTitle',
			'_yoast_wpseo_metadesc'          => 'seoDescription',
			'_yoast_wpseo_focuskw'           => 'seoFocusKeyword',
			'_yoast_wpseo_canonical'         => 'seoCanonical',
			'_yoast_wpseo_meta-robots-noindex'   => 'seoRobotsNoindex',
			'_yoast_wpseo_meta-robots-nofollow'  => 'seoRobotsNofollow',
			'_yoast_wpseo_opengraph-title'   => 'ogTitle',
			'_yoast_wpseo_opengraph-description' => 'ogDescription',
			'_yoast_wpseo_opengraph-image'   => 'ogImage',
			'_yoast_wpseo_twitter-title'     => 'twitterTitle',
			'_yoast_wpseo_twitter-description' => 'twitterDescription',
			'_yoast_wpseo_twitter-image'     => 'twitterImage',
		];
		$out = $this->pickKeys( $postMeta, $mapping );
		if ( count( $out ) > 0 ) {
			$out['seoSource'] = [ 'yoast' ];
		}
		return $out;
	}

	/**
	 * @param array<string,array<int,mixed>> $postMeta
	 * @return array<string,array<int,string>>
	 */
	private function extractFromRankMath( array $postMeta ): array {
		$mapping = [
			'rank_math_title'              => 'seoTitle',
			'rank_math_description'        => 'seoDescription',
			'rank_math_focus_keyword'      => 'seoFocusKeyword',
			'rank_math_canonical_url'      => 'seoCanonical',
			'rank_math_robots'             => 'seoRobots',
			'rank_math_facebook_title'     => 'ogTitle',
			'rank_math_facebook_description' => 'ogDescription',
			'rank_math_facebook_image'     => 'ogImage',
			'rank_math_twitter_title'      => 'twitterTitle',
			'rank_math_twitter_description' => 'twitterDescription',
			'rank_math_twitter_image'      => 'twitterImage',
		];
		$out = $this->pickKeys( $postMeta, $mapping );
		if ( count( $out ) > 0 ) {
			$out['seoSource'] = [ 'rankmath' ];
		}
		return $out;
	}

	/**
	 * @param array<string,array<int,mixed>> $postMeta
	 * @return array<string,array<int,string>>
	 */
	private function extractFromSeoPress( array $postMeta ): array {
		$mapping = [
			'_seopress_titles_title'       => 'seoTitle',
			'_seopress_titles_desc'        => 'seoDescription',
			'_seopress_analysis_target_kw' => 'seoFocusKeyword',
			'_seopress_robots_canonical'   => 'seoCanonical',
			'_seopress_social_fb_title'    => 'ogTitle',
			'_seopress_social_fb_desc'     => 'ogDescription',
			'_seopress_social_fb_img'      => 'ogImage',
			'_seopress_social_twitter_title' => 'twitterTitle',
			'_seopress_social_twitter_desc'  => 'twitterDescription',
			'_seopress_social_twitter_img'   => 'twitterImage',
		];
		$out = $this->pickKeys( $postMeta, $mapping );
		if ( count( $out ) > 0 ) {
			$out['seoSource'] = [ 'seopress' ];
		}
		return $out;
	}

	/**
	 * @param array<string,array<int,mixed>> $postMeta
	 * @param array<string,string>           $mapping `raw_meta_key => normalised_key`
	 * @return array<string,array<int,string>>
	 */
	private function pickKeys( array $postMeta, array $mapping ): array {
		$out = [];
		foreach ( $mapping as $rawKey => $cleanKey ) {
			if ( ! isset( $postMeta[ $rawKey ] ) ) {
				continue;
			}
			$values = $postMeta[ $rawKey ];
			if ( ! is_array( $values ) ) {
				$values = [ $values ];
			}
			$strings = [];
			foreach ( $values as $value ) {
				$flat = $this->stringify( $value );
				if ( $flat !== '' ) {
					$strings[] = $flat;
				}
			}
			if ( count( $strings ) > 0 ) {
				$out[ $cleanKey ] = $strings;
			}
		}
		return $out;
	}

	private function stringify( mixed $value ): string {
		if ( $value === null ) {
			return '';
		}
		if ( is_string( $value ) ) {
			return trim( $value );
		}
		if ( is_bool( $value ) ) {
			return $value ? '1' : '0';
		}
		if ( is_scalar( $value ) ) {
			return (string) $value;
		}
		$encoded = wp_json_encode( $value );
		return is_string( $encoded ) ? $encoded : '';
	}
}
