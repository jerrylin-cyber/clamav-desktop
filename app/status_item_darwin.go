//go:build darwin && cgo

package main

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

extern void StatusItemOpenWindow(void);
extern void StatusItemScanDownloads(void);
extern void StatusItemUpdateDatabase(void);
extern void StatusItemPauseSchedule(void);
extern void StatusItemShowLastResult(void);
extern void StatusItemQuit(void);

@interface ClamAVStatusItemDelegate : NSObject
- (void)openWindow:(id)sender;
- (void)scanDownloads:(id)sender;
- (void)updateDatabase:(id)sender;
- (void)pauseSchedule:(id)sender;
- (void)showLastResult:(id)sender;
- (void)quit:(id)sender;
@end

@implementation ClamAVStatusItemDelegate
- (void)openWindow:(id)sender { StatusItemOpenWindow(); }
- (void)scanDownloads:(id)sender { StatusItemScanDownloads(); }
- (void)updateDatabase:(id)sender { StatusItemUpdateDatabase(); }
- (void)pauseSchedule:(id)sender { StatusItemPauseSchedule(); }
- (void)showLastResult:(id)sender { StatusItemShowLastResult(); }
- (void)quit:(id)sender { StatusItemQuit(); }
@end

static NSStatusItem *clamAVStatusItem = nil;
static ClamAVStatusItemDelegate *clamAVStatusDelegate = nil;

static void addStatusMenuItem(NSMenu *menu, NSString *title, SEL action) {
	NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:title action:action keyEquivalent:@""];
	[item setTarget:clamAVStatusDelegate];
	[menu addItem:item];
}

static void StartClamAVStatusItem(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (clamAVStatusItem != nil) {
			return;
		}

		clamAVStatusDelegate = [ClamAVStatusItemDelegate new];
		clamAVStatusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
		clamAVStatusItem.button.toolTip = @"ClamAV Desktop";

		NSImage *icon = [[NSApp applicationIconImage] copy];
		[icon setSize:NSMakeSize(18, 18)];
		[icon setTemplate:NO];
		clamAVStatusItem.button.image = icon;

		NSMenu *menu = [NSMenu new];
		addStatusMenuItem(menu, @"開啟視窗", @selector(openWindow:));
		addStatusMenuItem(menu, @"掃描 Downloads", @selector(scanDownloads:));
		addStatusMenuItem(menu, @"更新病毒碼", @selector(updateDatabase:));
		addStatusMenuItem(menu, @"暫停排程", @selector(pauseSchedule:));
		addStatusMenuItem(menu, @"最近結果", @selector(showLastResult:));
		[menu addItem:[NSMenuItem separatorItem]];
		addStatusMenuItem(menu, @"結束", @selector(quit:));
		clamAVStatusItem.menu = menu;
	});
}
*/
import "C"

func nativeStartStatusItem() {
	C.StartClamAVStatusItem()
}
