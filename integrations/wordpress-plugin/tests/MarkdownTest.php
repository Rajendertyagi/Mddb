<?php
/**
 * Tests for the Markdown → HTML converter.
 *
 * @package Tradik\MddbSync\Tests
 */

declare(strict_types=1);

namespace Tradik\MddbSync\Tests;

use Brain\Monkey;
use PHPUnit\Framework\TestCase;
use Tradik\MddbSync\Markdown;

final class MarkdownTest extends TestCase {

	private Markdown $markdown;

	protected function setUp(): void {
		parent::setUp();
		Monkey\setUp();
		$this->markdown = new Markdown();
	}

	protected function tearDown(): void {
		Monkey\tearDown();
		parent::tearDown();
	}

	public function testHeadingsRenderAtEveryLevel(): void {
		self::assertSame( '<h1>Title</h1>', $this->markdown->toHtml( '# Title' ) );
		self::assertSame( '<h3>Sub</h3>', $this->markdown->toHtml( '### Sub' ) );
		self::assertSame( '<h6>Deep</h6>', $this->markdown->toHtml( '###### Deep' ) );
	}

	public function testParagraphWithInlineMarks(): void {
		self::assertSame(
			'<p><strong>b</strong> <em>i</em> <code>c</code></p>',
			$this->markdown->toHtml( '**b** *i* `c`' )
		);
	}

	public function testUnderscoreVariants(): void {
		self::assertSame(
			'<p><strong>b</strong> and <em>i</em></p>',
			$this->markdown->toHtml( '__b__ and _i_' )
		);
	}

	public function testMultilineParagraphIsJoined(): void {
		self::assertSame(
			'<p>line one line two</p>',
			$this->markdown->toHtml( "line one\nline two" )
		);
	}

	public function testUnorderedAndOrderedLists(): void {
		self::assertSame(
			'<ul><li>alpha</li><li>beta</li></ul>',
			$this->markdown->toHtml( "- alpha\n- beta" )
		);
		self::assertSame(
			'<ol><li>one</li><li>two</li></ol>',
			$this->markdown->toHtml( "1. one\n2. two" )
		);
	}

	public function testListContinuationLinesAttachToPreviousItem(): void {
		self::assertSame(
			'<ul><li>alpha continued</li></ul>',
			$this->markdown->toHtml( "- alpha\n  continued" )
		);
	}

	public function testBlockquoteAndHorizontalRule(): void {
		self::assertSame(
			'<blockquote><p>wise words</p></blockquote>',
			$this->markdown->toHtml( '> wise words' )
		);
		self::assertSame( '<hr />', $this->markdown->toHtml( '---' ) );
	}

	public function testLinksAndImages(): void {
		self::assertSame(
			'<p><a href="https://example.com/a">go</a></p>',
			$this->markdown->toHtml( '[go](https://example.com/a)' )
		);
		self::assertSame(
			'<p><img src="https://example.com/i.png" alt="pic" /></p>',
			$this->markdown->toHtml( '![pic](https://example.com/i.png)' )
		);
	}

	public function testFencedCodeBlockKeepsContentVerbatimAndEscaped(): void {
		$html = $this->markdown->toHtml( "```php\necho \"<b>\";\n```" );
		self::assertSame(
			'<pre><code class="language-php">echo &quot;&lt;b&gt;&quot;;' . "\n" . '</code></pre>',
			$html
		);
	}

	public function testFencedCodeBlockWithoutLanguage(): void {
		$html = $this->markdown->toHtml( "```\n**not bold**\n```" );
		self::assertStringContainsString( '**not bold**', $html );
		self::assertStringNotContainsString( '<strong>', $html );
	}

	public function testRawHtmlIsEscapedNotExecuted(): void {
		$html = $this->markdown->toHtml( '<script>alert("x")</script>' );
		self::assertStringNotContainsString( '<script>', $html );
		self::assertStringContainsString( '&lt;script&gt;', $html );
	}

	public function testInlineCodeShieldsMarksFromEmphasis(): void {
		self::assertSame(
			'<p><code>*raw*</code></p>',
			$this->markdown->toHtml( '`*raw*`' )
		);
	}

	public function testCrlfInputIsNormalised(): void {
		self::assertSame(
			"<h2>A</h2>\n<p>b</p>",
			$this->markdown->toHtml( "## A\r\n\r\nb" )
		);
	}

	public function testEmptyInputYieldsEmptyOutput(): void {
		self::assertSame( '', $this->markdown->toHtml( '' ) );
		self::assertSame( '', $this->markdown->toHtml( "\n\n" ) );
	}
}
