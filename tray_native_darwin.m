#import <Cocoa/Cocoa.h>
#import <dispatch/dispatch.h>

extern void ecsDarwinTrayAction(char *kind, char *profile);
static NSStatusItem *ecsStatusItem;
static id ecsDelegate;

@interface ECSMenuDelegate : NSObject
- (void)performAction:(id)sender;
@end
@implementation ECSMenuDelegate
- (void)performAction:(id)sender {
    NSDictionary *action = [sender representedObject];
    ecsDarwinTrayAction((char *)[[action objectForKey:@"kind"] UTF8String], (char *)[[action objectForKey:@"profile"] UTF8String]);
}
@end

static void ecsOnMain(dispatch_block_t block) {
    if ([NSThread isMainThread]) { block(); } else { dispatch_sync(dispatch_get_main_queue(), block); }
}
static NSMenuItem *ecsItem(NSString *title, NSString *kind, NSString *profile, BOOL enabled) {
    NSMenuItem *item = [[[NSMenuItem alloc] initWithTitle:title action:@selector(performAction:) keyEquivalent:@""] autorelease];
    [item setTarget:ecsDelegate];
    [item setRepresentedObject:@{@"kind": kind, @"profile": profile}];
    [item setEnabled:enabled];
    return item;
}

int ecs_tray_start(void) {
    __block int ok = 0;
    ecsOnMain(^{
        ecsDelegate = [[ECSMenuDelegate alloc] init];
        ecsStatusItem = [[[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength] retain];
        if (ecsStatusItem != nil) {
            [[ecsStatusItem button] setTitle:@"VPN"];
            ok = 1;
        }
    });
    return ok;
}
void ecs_tray_update(const char *json) {
    NSString *value = [[NSString alloc] initWithUTF8String:json];
    dispatch_async(dispatch_get_main_queue(), ^{
        if (ecsStatusItem != nil) {
            NSData *data = [value dataUsingEncoding:NSUTF8StringEncoding];
            NSDictionary *snapshot = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
            if (snapshot != nil) {
                NSMenu *menu = [[[NSMenu alloc] initWithTitle:@"VPN"] autorelease];
                [menu setAutoenablesItems:NO];
                [menu addItem:ecsItem(snapshot[@"summary"], @"none", @"", NO)];
                [menu addItem:ecsItem(@"打开 VPN 工作台", @"open", @"", YES)];
                [menu addItem:[NSMenuItem separatorItem]];
                for (NSDictionary *profile in snapshot[@"profiles"]) {
                    NSMenuItem *item = [[[NSMenuItem alloc] initWithTitle:profile[@"title"] action:nil keyEquivalent:@""] autorelease];
                    NSMenu *submenu = [[[NSMenu alloc] initWithTitle:profile[@"title"]] autorelease];
                    [submenu setAutoenablesItems:NO];
                    [submenu addItem:ecsItem(profile[@"label"], profile[@"action"], profile[@"id"], ![profile[@"disabled"] boolValue])];
                    [submenu addItem:ecsItem(@"查看详情", @"details", profile[@"id"], YES)];
                    [item setSubmenu:submenu];
                    [menu addItem:item];
                }
                [menu addItem:[NSMenuItem separatorItem]];
                [menu addItem:ecsItem(@"断开全部 VPN", @"disconnect_all", @"", [snapshot[@"running"] boolValue])];
                [menu addItem:ecsItem(@"退出程序（断开全部）", @"quit", @"", YES)];
                [ecsStatusItem setMenu:menu];
                [[ecsStatusItem button] setToolTip:snapshot[@"summary"]];
            }
        }
        [value release];
    });
}
void ecs_tray_remove(void) {
    ecsOnMain(^{
        if (ecsStatusItem != nil) {
            [[NSStatusBar systemStatusBar] removeStatusItem:ecsStatusItem];
            [ecsStatusItem release];
            ecsStatusItem = nil;
        }
        [ecsDelegate release];
        ecsDelegate = nil;
    });
}
