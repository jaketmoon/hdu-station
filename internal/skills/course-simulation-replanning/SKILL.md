---
name: course-simulation-replanning
description: 把已有模拟加入课程作为待选集合，保留其他课程，每门最多选一个班，计算无冲突组合并按用户要求一次性保存。适用于“这些课每门一个班、自由组合不冲突”等已有模拟课程重排请求。
---
先用 manage_course_simulation 的 read 读取目标学期，结合上下文定位用户指定的完整待选集合，将该集合所有 ENROLL 班级的 classID 传给 plan_course_simulation。同一门的多个班都是备选，不是固定占用。指代不清且会误删其他模拟课程时才澄清；已有明确的保存授权无需再次询问。

plan_course_simulation 在内存中释放待选集合占用，按全部周次、星期、节次计算每课程号最多一班的组合。真实已选课程全部固定占位、不可删除，即使已有模拟 DROP 也不据此腾出重排空间；集合外模拟加入继续占位，集合外所有模拟动作原样保留。工具只接受当前模拟加入班级，不接受真实课程、DROP 或临时编造的班号。已有班级从最新模拟记录取证，无需重新搜索开课。

只采用工具的 suggestedPlan，不把 fits 列表当成互不冲突组合，不要求所有课程都能排入。时间未知的待选班级不进入组合；固定课程有冲突或未知时间时工具阻止生成修改。planIncomplete=true 表示达到12门或搜索步数上限，可说明得到可行组合但未证明门数最多。

用户要求保存且 status=planned 时，将返回的 update 参数原样传给 manage_course_simulation，一次 update 撤销未入选的模拟加入，保留选中的班和其他内容。必须保留 expectedRevision、目标学期和 requireNoConflicts=true。不要先删除候选再插空，不用 replace/reset，不新增 DROP，不为“排得下”修改真实课程或集合外课程。版本变化或 unknown 时停止，不能用新 revision 自动重放旧方案。

仅咨询方案时不写。保存后仅依据 confirmed 或 unchanged 报告成功，简列保留课程及未入选/时间未知项；blocked 或未确认时说明具体缺口，不声称已经保存或保证无冲突。新增非现有班级属于开课核实与模拟管理能力，不混入本次已有集合重排。
