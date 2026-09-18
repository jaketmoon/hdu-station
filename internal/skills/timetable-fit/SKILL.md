---
name: timetable-fit
description: 以 Neo 模拟课表补空、核对冲突和时间偏好；明确查看真实课表时才切换来源。
---
使用 fit_courses_to_schedule，补空、插空和默认冲突筛选传 scheduleSource="simulation"，只依据 effectiveCourses：模拟加入占用时间，模拟移除释放时间，保留的真实课程继续占位。不能用 actualCourses 或真实课表替代；模拟读取失败或时间不完整时说明无法确认，不回退。只有用户明确要查看学校真实课表时传 scheduleSource="actual"；之后补空仍切回 simulation。仅问本人课表或空闲时段时省略 courses，问局部课表也传 allowedDays、timeOfDay 或 allowedSections，展示该范围的占用与空闲；不搜索推荐，也不要求选定课程。问指定课程能否放入时传课程名或沿用已查候选；未知开课时间可组合 offering-verification，工具支持一次查询并核对。

时间约定：上午为第1–5节，下午为第6–9节，晚上为第10–13节。用 timeOfDay 传 morning/afternoon/evening，由程序展开；明确节次用 allowedSections，明确星期用 allowedDays。多个时段可并集，同传具体节次时取交集。没有官方钟点对照时不猜节次。条件含糊或矛盾时先澄清；只传明确偏好，不把已有课的占用当作允许时段。

按全部周次、星期、节次判断，采用工具结论；缺页或解析失败不能确认空闲。fits 仅指与当前模拟课表有效课程兼容，多门一起安排使用 suggestedPlan。追问保留课程和仍有效的限制，用户修改偏好则覆盖旧限制；指代某个班时结合前文的课程与上课时间定位。

直接根据结构化结果总结忙闲或课程适配；需要进一步筛选已有结果时可用show_course_results，不会结束本轮。scheduleSource与核对时一致，补空为simulation；只查课表无需提供课程。完成条件随问题而定：看课表得到占用与完整性说明即可；核对课程则给出可放入、冲突、不合偏好或待确认及原因。只展示课程名、课程号、老师和时间，不展示班级号。

本工具只读取当前模拟课表并计算适配，不修改或保存模拟方案，也不能释放已有课程占用。模拟管理与重排是否可用以当前工具和能力说明为准。
